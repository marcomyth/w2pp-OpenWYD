package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

const (
	legacyGuildPerServer = 4096
	guildAllocLockBase   = 0x77326775696c6400
)

// CreateGuild allocates a legacy ushort guild id, creates the guild row, debits
// the creation cost, and makes the requesting character its leader in one
// transaction.
func (s *Store) CreateGuild(ctx context.Context, accountID int64, slot int, characterName, guildName string, clan, citizen uint8, serverIndex int, cost int32) (domain.Guild, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Guild{}, fmt.Errorf("store: begin create guild: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if serverIndex < 0 {
		serverIndex = 0
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(guildAllocLockBase+serverIndex)); err != nil {
		return domain.Guild{}, fmt.Errorf("store: lock guild allocator: %w", err)
	}

	var charID int64
	var currentGuild int
	var coin int32
	err = tx.QueryRow(ctx, `
		SELECT id, guild_id, coin
		  FROM character
		 WHERE account_id = $1 AND slot = $2 AND name = $3
		 FOR UPDATE`,
		accountID, slot, characterName,
	).Scan(&charID, &currentGuild, &coin)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Guild{}, ErrNotFound
	}
	if err != nil {
		return domain.Guild{}, fmt.Errorf("store: lock character for guild create: %w", err)
	}
	if currentGuild != 0 || cost < 0 || coin < cost {
		return domain.Guild{}, ErrConflict
	}

	minID := serverIndex*legacyGuildPerServer + 1
	maxID := serverIndex*legacyGuildPerServer + legacyGuildPerServer - 1
	var guildID int
	err = tx.QueryRow(ctx,
		`SELECT COALESCE(MAX(id), $1 - 1) + 1 FROM guild WHERE id BETWEEN $1 AND $2`,
		minID, maxID,
	).Scan(&guildID)
	if err != nil {
		return domain.Guild{}, fmt.Errorf("store: allocate guild id: %w", err)
	}
	if guildID > maxID || guildID >= 65536 {
		return domain.Guild{}, ErrNoFreeSlot
	}

	guild := domain.Guild{ID: uint16(guildID), Name: guildName, Clan: clan, Citizen: citizen}
	if _, err := tx.Exec(ctx, `
		INSERT INTO guild(id, name, clan, fame, citizen)
		VALUES ($1, $2, $3, 0, $4)`,
		guildID, guildName, clan, citizen,
	); err != nil {
		return domain.Guild{}, fmt.Errorf("store: insert guild %q: %w", guildName, err)
	}
	if err := setGuildMemberTx(ctx, tx, accountID, slot, charID, characterName, uint16(guildID), 9); err != nil {
		return domain.Guild{}, err
	}
	if err := spendCharacterCoinTx(ctx, tx, charID, cost); err != nil {
		return domain.Guild{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Guild{}, fmt.Errorf("store: commit create guild %q: %w", guildName, err)
	}
	return guild, nil
}

// SetGuildMember sets a character's guild id/level and mirrors it in
// guild_member. A guild id of 0 removes the membership.
func (s *Store) SetGuildMember(ctx context.Context, accountID int64, slot int, characterName string, guildID uint16, guildLevel uint8) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("store: begin set guild member: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var charID int64
	err = tx.QueryRow(ctx, `
		SELECT id FROM character
		 WHERE account_id = $1 AND slot = $2 AND name = $3
		 FOR UPDATE`,
		accountID, slot, characterName,
	).Scan(&charID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("store: lock character for guild member: %w", err)
	}
	if err := setGuildMemberTx(ctx, tx, accountID, slot, charID, characterName, guildID, guildLevel); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("store: commit set guild member: %w", err)
	}
	return nil
}

// LeaveGuild removes a character from its guild.
func (s *Store) LeaveGuild(ctx context.Context, accountID int64, slot int) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("store: begin leave guild: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var charID int64
	err = tx.QueryRow(ctx,
		`SELECT id FROM character WHERE account_id = $1 AND slot = $2 FOR UPDATE`,
		accountID, slot,
	).Scan(&charID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("store: lock character for leave guild: %w", err)
	}
	if err := setGuildMemberTx(ctx, tx, accountID, slot, charID, "", 0, 0); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("store: commit leave guild: %w", err)
	}
	return nil
}

// PromoteGuildMember assigns the first available sub-leader level (6, 7, 8) and
// debits the leader in the same transaction.
func (s *Store) PromoteGuildMember(ctx context.Context, guildID uint16, leaderAccountID int64, leaderSlot int, accountID int64, slot int, cost int32) (uint8, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("store: begin promote guild member: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	leaderID, _, leaderGuild, leaderLevel, leaderCoin, err := lockGuildCharacterWithCoin(ctx, tx, leaderAccountID, leaderSlot)
	if err != nil {
		return 0, err
	}
	if leaderGuild != int(guildID) || leaderLevel != 9 || cost < 0 || leaderCoin < cost {
		return 0, ErrConflict
	}

	var charID int64
	var name string
	var currentGuild int
	var currentLevel int
	err = tx.QueryRow(ctx, `
		SELECT id, name, guild_id, guild_level
		  FROM character
		 WHERE account_id = $1 AND slot = $2
		 FOR UPDATE`,
		accountID, slot,
	).Scan(&charID, &name, &currentGuild, &currentLevel)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("store: lock character for guild promotion: %w", err)
	}
	if currentGuild != int(guildID) || currentLevel != 0 {
		return 0, ErrConflict
	}

	rows, err := tx.Query(ctx, `
		SELECT guild_level FROM guild_member
		 WHERE guild_id = $1 AND guild_level BETWEEN 6 AND 8
		 FOR UPDATE`,
		guildID,
	)
	if err != nil {
		return 0, fmt.Errorf("store: list guild subleaders: %w", err)
	}
	used := [3]bool{}
	for rows.Next() {
		var level int
		if err := rows.Scan(&level); err != nil {
			rows.Close()
			return 0, fmt.Errorf("store: scan guild subleader: %w", err)
		}
		if level >= 6 && level <= 8 {
			used[level-6] = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("store: list guild subleaders: %w", err)
	}
	rows.Close()

	level := uint8(0)
	for i, ok := range used {
		if !ok {
			level = uint8(6 + i)
			break
		}
	}
	if level == 0 {
		return 0, ErrNoFreeSlot
	}
	if err := spendCharacterCoinTx(ctx, tx, leaderID, cost); err != nil {
		return 0, err
	}
	if err := setGuildMemberTx(ctx, tx, accountID, slot, charID, name, guildID, level); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("store: commit promote guild member: %w", err)
	}
	return level, nil
}

// TransferGuildLeader moves level 9 from oldAccountID/slot to newAccountID/slot.
func (s *Store) TransferGuildLeader(ctx context.Context, guildID uint16, oldAccountID int64, oldSlot int, newAccountID int64, newSlot int) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("store: begin transfer guild leader: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	oldID, oldName, oldGuild, oldLevel, err := lockGuildCharacter(ctx, tx, oldAccountID, oldSlot)
	if err != nil {
		return err
	}
	newID, newName, newGuild, _, err := lockGuildCharacter(ctx, tx, newAccountID, newSlot)
	if err != nil {
		return err
	}
	if oldGuild != int(guildID) || newGuild != int(guildID) || oldLevel != 9 {
		return ErrConflict
	}
	if err := setGuildMemberTx(ctx, tx, oldAccountID, oldSlot, oldID, oldName, guildID, 0); err != nil {
		return err
	}
	if err := setGuildMemberTx(ctx, tx, newAccountID, newSlot, newID, newName, guildID, 9); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("store: commit transfer guild leader: %w", err)
	}
	return nil
}

func lockGuildCharacter(ctx context.Context, tx pgx.Tx, accountID int64, slot int) (int64, string, int, int, error) {
	var charID int64
	var name string
	var guildID, guildLevel int
	err := tx.QueryRow(ctx, `
		SELECT id, name, guild_id, guild_level
		  FROM character
		 WHERE account_id = $1 AND slot = $2
		 FOR UPDATE`,
		accountID, slot,
	).Scan(&charID, &name, &guildID, &guildLevel)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", 0, 0, ErrNotFound
	}
	if err != nil {
		return 0, "", 0, 0, fmt.Errorf("store: lock guild character: %w", err)
	}
	return charID, name, guildID, guildLevel, nil
}

func lockGuildCharacterWithCoin(ctx context.Context, tx pgx.Tx, accountID int64, slot int) (int64, string, int, int, int32, error) {
	var charID int64
	var name string
	var guildID, guildLevel int
	var coin int32
	err := tx.QueryRow(ctx, `
		SELECT id, name, guild_id, guild_level, coin
		  FROM character
		 WHERE account_id = $1 AND slot = $2
		 FOR UPDATE`,
		accountID, slot,
	).Scan(&charID, &name, &guildID, &guildLevel, &coin)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", 0, 0, 0, ErrNotFound
	}
	if err != nil {
		return 0, "", 0, 0, 0, fmt.Errorf("store: lock guild character with coin: %w", err)
	}
	return charID, name, guildID, guildLevel, coin, nil
}

func spendCharacterCoinTx(ctx context.Context, tx pgx.Tx, charID int64, cost int32) error {
	if cost == 0 {
		return nil
	}
	tag, err := tx.Exec(ctx, `UPDATE character SET coin = coin - $2 WHERE id = $1 AND coin >= $2`, charID, cost)
	if err != nil {
		return fmt.Errorf("store: spend character coin: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

func setGuildMemberTx(ctx context.Context, tx pgx.Tx, accountID int64, slot int, charID int64, name string, guildID uint16, guildLevel uint8) error {
	if _, err := tx.Exec(ctx,
		`UPDATE character SET guild_id = $3, guild_level = $4 WHERE account_id = $1 AND slot = $2`,
		accountID, slot, guildID, guildLevel,
	); err != nil {
		return fmt.Errorf("store: update character guild fields: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM guild_member WHERE character_id = $1`, charID); err != nil {
		return fmt.Errorf("store: clear guild member: %w", err)
	}
	if guildID == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO guild_member(guild_id, character_id, account_id, slot, name, guild_level, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, now())`,
		guildID, charID, accountID, slot, name, guildLevel,
	); err != nil {
		return fmt.Errorf("store: insert guild member: %w", err)
	}
	return nil
}

// SetGuildRelation upserts or removes one directed ally/war relation.
func (s *Store) SetGuildRelation(ctx context.Context, guildID, targetGuildID uint16, kind domain.GuildRelationKind) error {
	if guildID == 0 || guildID == targetGuildID {
		return ErrConflict
	}
	if kind == domain.GuildRelationNone || targetGuildID == 0 {
		tag, err := s.pool.Exec(ctx,
			`DELETE FROM guild_relation WHERE guild_id = $1 AND ($2 = 0 OR kind = $2)`, guildID, kind)
		if err != nil {
			return fmt.Errorf("store: clear guild relation: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO guild_relation(guild_id, target_guild_id, kind, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (guild_id, kind)
		DO UPDATE SET target_guild_id = EXCLUDED.target_guild_id, updated_at = now()`,
		guildID, targetGuildID, kind,
	)
	if err != nil {
		return fmt.Errorf("store: set guild relation: %w", err)
	}
	return nil
}

// ListGuilds returns every registered guild ordered by id.
func (s *Store) ListGuilds(ctx context.Context) ([]domain.Guild, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, clan, fame, citizen, notice, notice_by, notice_at, member_cap
		   FROM guild ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("store: list guilds: %w", err)
	}
	defer rows.Close()
	var out []domain.Guild
	for rows.Next() {
		var g domain.Guild
		// notice_at é o único nulável da linha: ele fica nulo enquanto ninguém
		// escreveu recado, e é assim que o painel sabe não desenhar a data.
		var noticeAt *time.Time
		if err := rows.Scan(&g.ID, &g.Name, &g.Clan, &g.Fame, &g.Citizen,
			&g.Notice, &g.NoticeBy, &noticeAt, &g.MemberCap); err != nil {
			return nil, fmt.Errorf("store: scan guild: %w", err)
		}
		if noticeAt != nil {
			g.NoticeAt = *noticeAt
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// SaveGuildNotice grava o recado da guilda e carimba a hora e o autor.
//
// A hora é do BANCO (now()), não de quem chamou: o carimbo aparece para todo
// mundo que abre o painel, e o relógio do tmServer não é o mesmo do banco.
func (s *Store) SaveGuildNotice(ctx context.Context, guildID uint16, notice, by string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE guild
		   SET notice = $2, notice_by = $3, notice_at = now(), updated_at = now()
		 WHERE id = $1`, int32(guildID), notice, by)
	if err != nil {
		return fmt.Errorf("store: save guild notice of %d: %w", guildID, err)
	}
	return nil
}

// ListGuildRelations returns every directed guild relation ordered by guild id.
func (s *Store) ListGuildRelations(ctx context.Context) ([]domain.GuildRelation, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT guild_id, target_guild_id, kind FROM guild_relation ORDER BY guild_id, target_guild_id`)
	if err != nil {
		return nil, fmt.Errorf("store: list guild relations: %w", err)
	}
	defer rows.Close()
	var out []domain.GuildRelation
	for rows.Next() {
		var r domain.GuildRelation
		if err := rows.Scan(&r.GuildID, &r.TargetGuildID, &r.Kind); err != nil {
			return nil, fmt.Errorf("store: scan guild relation: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// LoadGuildZones loads the five guild/city zones.
func (s *Store) LoadGuildZones(ctx context.Context) ([]domain.GuildZone, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT zone, charge_guild, challenge_guild, clan, victory, city_tax, challenge_money, tax_vault,
		       guild_spawn_x, guild_spawn_y
		  FROM guild_zone ORDER BY zone`)
	if err != nil {
		return nil, fmt.Errorf("store: load guild zones: %w", err)
	}
	defer rows.Close()
	var out []domain.GuildZone
	for rows.Next() {
		var z domain.GuildZone
		if err := rows.Scan(&z.Zone, &z.ChargeGuild, &z.ChallengeGuild, &z.Clan, &z.Victory, &z.CityTax, &z.ChallengeMoney, &z.TaxVault,
			&z.GuildSpawnX, &z.GuildSpawnY); err != nil {
			return nil, fmt.Errorf("store: scan guild zone: %w", err)
		}
		out = append(out, z)
	}
	return out, rows.Err()
}

// SaveGuildZone persists one guild/city zone.
func (s *Store) SaveGuildZone(ctx context.Context, z domain.GuildZone) error {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO guild_zone(zone, charge_guild, challenge_guild, clan, victory, city_tax, challenge_money, tax_vault,
		                       guild_spawn_x, guild_spawn_y, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
		ON CONFLICT (zone)
		DO UPDATE SET charge_guild = EXCLUDED.charge_guild,
		              challenge_guild = EXCLUDED.challenge_guild,
		              clan = EXCLUDED.clan,
		              victory = EXCLUDED.victory,
		              city_tax = EXCLUDED.city_tax,
		              challenge_money = EXCLUDED.challenge_money,
		              tax_vault = EXCLUDED.tax_vault,
		              guild_spawn_x = EXCLUDED.guild_spawn_x,
		              guild_spawn_y = EXCLUDED.guild_spawn_y,
		              updated_at = now()`,
		z.Zone, z.ChargeGuild, z.ChallengeGuild, z.Clan, z.Victory, z.CityTax, z.ChallengeMoney, z.TaxVault,
		z.GuildSpawnX, z.GuildSpawnY,
	)
	if err != nil {
		return fmt.Errorf("store: save guild zone %d: %w", z.Zone, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// LoadGuildTowerState loads the single GTorre owner row.
func (s *Store) LoadGuildTowerState(ctx context.Context) (domain.GuildTowerState, error) {
	var st domain.GuildTowerState
	err := s.pool.QueryRow(ctx,
		`SELECT owner_guild, updated_at_unix FROM guild_tower_state WHERE id = 1`,
	).Scan(&st.OwnerGuild, &st.UpdatedAtUnix)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.GuildTowerState{}, ErrNotFound
	}
	if err != nil {
		return domain.GuildTowerState{}, fmt.Errorf("store: load guild tower state: %w", err)
	}
	return st, nil
}

// SaveGuildTowerState persists the single GTorre owner row.
func (s *Store) SaveGuildTowerState(ctx context.Context, st domain.GuildTowerState) error {
	if st.UpdatedAtUnix == 0 {
		st.UpdatedAtUnix = time.Now().Unix()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO guild_tower_state(id, owner_guild, updated_at_unix)
		VALUES (1, $1, $2)
		ON CONFLICT (id)
		DO UPDATE SET owner_guild = EXCLUDED.owner_guild,
		              updated_at_unix = EXCLUDED.updated_at_unix`,
		st.OwnerGuild, st.UpdatedAtUnix,
	)
	if err != nil {
		return fmt.Errorf("store: save guild tower state: %w", err)
	}
	return nil
}

// UpdateGuildFame writes a guild's fame. It is an absolute value, not a delta:
// tmServer owns the live number (World.SetGuildFame) and this only keeps the
// database in step with it, so a retried write cannot award the fame twice.
//
// Until this existed the store only ever INSERTed a guild, so fame earned in
// game — the Tower War's +100, a GM's /gm guildfame — lived in memory and was
// gone at the next restart. ErrNotFound means no guild has that id.
func (s *Store) UpdateGuildFame(ctx context.Context, guildID uint16, fame int32) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE guild SET fame = $2, updated_at = now() WHERE id = $1`,
		int32(guildID), fame)
	if err != nil {
		return fmt.Errorf("store: update guild %d fame: %w", guildID, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// LoadCastleQuestState loads the single Castle/Zakum quest row.
func (s *Store) LoadCastleQuestState(ctx context.Context) (domain.CastleQuestState, error) {
	var st domain.CastleQuestState
	err := s.pool.QueryRow(ctx,
		`SELECT level, time_left, clear, leader_name FROM castle_quest_state WHERE id = 1`,
	).Scan(&st.Level, &st.TimeLeft, &st.Clear, &st.LeaderName)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CastleQuestState{}, ErrNotFound
	}
	if err != nil {
		return domain.CastleQuestState{}, fmt.Errorf("store: load castle quest state: %w", err)
	}
	return st, nil
}

// SaveCastleQuestState persists the single Castle/Zakum quest row.
func (s *Store) SaveCastleQuestState(ctx context.Context, st domain.CastleQuestState) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO castle_quest_state(id, level, time_left, clear, leader_name, updated_at)
		VALUES (1, $1, $2, $3, $4, now())
		ON CONFLICT (id)
		DO UPDATE SET level = EXCLUDED.level,
		              time_left = EXCLUDED.time_left,
		              clear = EXCLUDED.clear,
		              leader_name = EXCLUDED.leader_name,
		              updated_at = now()`,
		st.Level, st.TimeLeft, st.Clear, st.LeaderName,
	)
	if err != nil {
		return fmt.Errorf("store: save castle quest state: %w", err)
	}
	return nil
}

// ListGuildMembers returns one guild's roster, leader first.
//
// The account name is joined in because it is the only thing on the row a
// moderator can act on: guild_member names a CHARACTER, and every panel action —
// ban, kick, deliver — works in accounts.
func (s *Store) ListGuildMembers(ctx context.Context, guildID uint16) ([]domain.GuildMember, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT m.guild_id, m.character_id, m.account_id, coalesce(a.name, ''),
		       m.slot, m.name, m.guild_level, m.status, m.last_seen
		  FROM guild_member m
		  LEFT JOIN account a ON a.id = m.account_id
		 WHERE m.guild_id = $1
		 ORDER BY m.guild_level DESC, m.name`, int32(guildID))
	if err != nil {
		return nil, fmt.Errorf("store: list guild members of %d: %w", guildID, err)
	}
	defer rows.Close()

	var out []domain.GuildMember
	for rows.Next() {
		var m domain.GuildMember
		var gid int32
		var lastSeen *time.Time
		if err := rows.Scan(&gid, &m.CharacterID, &m.AccountID, &m.AccountName,
			&m.Slot, &m.Name, &m.Level, &m.Status, &lastSeen); err != nil {
			return nil, fmt.Errorf("store: scan guild member: %w", err)
		}
		m.GuildID = uint16(gid)
		if lastSeen != nil {
			m.LastSeen = *lastSeen
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate guild members: %w", err)
	}
	return out, nil
}

// CountGuildMembers returns how many characters each guild holds, keyed by guild
// id.
//
// One query for every guild rather than one per guild: the list page shows the
// count on every row, and a roster read per row is how a list of forty guilds
// becomes forty round trips.
func (s *Store) CountGuildMembers(ctx context.Context) (map[uint16]int, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT guild_id, count(*) FROM guild_member GROUP BY guild_id`)
	if err != nil {
		return nil, fmt.Errorf("store: count guild members: %w", err)
	}
	defer rows.Close()

	out := map[uint16]int{}
	for rows.Next() {
		var gid int32
		var n int
		if err := rows.Scan(&gid, &n); err != nil {
			return nil, fmt.Errorf("store: scan guild member count: %w", err)
		}
		out[uint16(gid)] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate guild member counts: %w", err)
	}
	return out, nil
}

// ListGuildBuffs devolve os buffs de guilda que AINDA valem.
//
// O filtro é do banco (`expires_at > now()`) e não de quem chama: o relógio que
// decidiu a validade tem de ser o mesmo que a gravou, senão um tmServer com o
// relógio adiantado ressuscita buff vencido no boot.
func (s *Store) ListGuildBuffs(ctx context.Context) ([]domain.GuildBuff, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT guild_id, buff_type, expires_at
		  FROM guild_buff
		 WHERE expires_at > now()
		 ORDER BY guild_id, buff_type`)
	if err != nil {
		return nil, fmt.Errorf("store: list guild buffs: %w", err)
	}
	defer rows.Close()
	var out []domain.GuildBuff
	for rows.Next() {
		var b domain.GuildBuff
		var gid int32
		var tipo int16
		if err := rows.Scan(&gid, &tipo, &b.ExpiresAt); err != nil {
			return nil, fmt.Errorf("store: scan guild buff: %w", err)
		}
		b.GuildID, b.Type = uint16(gid), uint8(tipo)
		out = append(out, b)
	}
	return out, rows.Err()
}

// SaveGuildBuff grava até quando um buff vale, criando ou substituindo a linha.
func (s *Store) SaveGuildBuff(ctx context.Context, b domain.GuildBuff) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO guild_buff (guild_id, buff_type, expires_at, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (guild_id, buff_type)
		DO UPDATE SET expires_at = EXCLUDED.expires_at, updated_at = now()`,
		int32(b.GuildID), int16(b.Type), b.ExpiresAt)
	if err != nil {
		return fmt.Errorf("store: save guild buff %d/%d: %w", b.GuildID, b.Type, err)
	}
	return nil
}

// DeleteGuildBuff apaga a linha de um buff que venceu.
//
// Apagar em vez de deixar vencer sozinho no banco é o que impede a tabela de
// virar um histórico: ninguém pergunta quais buffs uma guilda já teve, e a
// leitura do boot ficaria mais cara a cada mês que passa.
func (s *Store) DeleteGuildBuff(ctx context.Context, guildID uint16, tipo uint8) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM guild_buff WHERE guild_id = $1 AND buff_type = $2`,
		int32(guildID), int16(tipo))
	if err != nil {
		return fmt.Errorf("store: delete guild buff %d/%d: %w", guildID, tipo, err)
	}
	return nil
}

// ListGuildSummaries devolve as guildas do servidor para a tela "Guilds do
// Server", as de maior fama primeiro.
//
// UMA consulta para a lista inteira, e é esse o ponto. O caminho óbvio — listar
// as guildas e, para cada uma, contar membros e achar o líder — são duas idas
// ao banco POR LINHA, ou cento e vinte round trips para uma tela de sessenta.
// A contagem vem de um agrupamento feito de uma vez, e o líder de um LATERAL
// que pára na primeira linha de cargo 9.
//
// A guilda sem líder gravado aparece com o nome vazio em vez de sumir da lista:
// ela existe, e esconder do jogador uma guilda que existe é pior do que mostrar
// um campo em branco.
func (s *Store) ListGuildSummaries(ctx context.Context, limite int) ([]domain.GuildSummary, error) {
	if limite <= 0 {
		limite = 60
	}
	rows, err := s.pool.Query(ctx, `
		SELECT g.id, g.name, g.fame,
		       coalesce(c.n, 0),
		       coalesce(l.name, '')
		  FROM guild g
		  LEFT JOIN (SELECT guild_id, count(*) AS n FROM guild_member GROUP BY guild_id) c
		         ON c.guild_id = g.id
		  LEFT JOIN LATERAL (
		         SELECT name FROM guild_member
		          WHERE guild_id = g.id AND guild_level = 9
		          LIMIT 1) l ON true
		 ORDER BY g.fame DESC, g.name
		 LIMIT $1`, limite)
	if err != nil {
		return nil, fmt.Errorf("store: list guild summaries: %w", err)
	}
	defer rows.Close()
	var out []domain.GuildSummary
	for rows.Next() {
		var g domain.GuildSummary
		var id int32
		if err := rows.Scan(&id, &g.Name, &g.Fame, &g.Members, &g.Leader); err != nil {
			return nil, fmt.Errorf("store: scan guild summary: %w", err)
		}
		g.ID = uint16(id)
		out = append(out, g)
	}
	return out, rows.Err()
}

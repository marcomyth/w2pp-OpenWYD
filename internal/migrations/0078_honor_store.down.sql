-- Volta o nome do NPC ao do template.
UPDATE npc_definition
   SET display_name = 'God_of_War', updated_at = now()
 WHERE template_name = 'God_of_War'
   AND display_name = 'Honor Store';

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;

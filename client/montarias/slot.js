#!/usr/bin/env node
// Cria uma montaria NOVA no cliente, sem patch no executável.
//
// COMO ISSO É POSSÍVEL
//
// A cadeia em 0x50C1E7 aceita o índice do item em Equip[14] nas faixas
// 2360–2389, 2960–2999 e 3980–3998, e o traduz num índice SUBSTITUTO. Fora dos
// casos nomeados, a regra é `substituto = índice − 2045`. Os índices 3995 a
// 3998 caem nessa regra e ninguém os usa.
//
// E o substituto não precisa ser uma das 32 linhas de sempre. Dele o cliente
// tira duas coisas, as duas do CATÁLOGO:
//
//     EF_CLASS (efeito 18)  → a FAMÍLIA do bicho
//         22 porco · 27 lobo · 16 dragão menor · 28 urso · 35 dente de sabre
//         42 fenrir · 43 cavalo · 51 TIGRE · 52 dragão vermelho · 53 grifo
//
//     campo +64 (mesh)      → a VARIANTE, e o valor é o NÚMERO DO ARQUIVO
//                             MENOS UM: 0 → tg010101, 12 → tg010113
//
// Confirmado em jogo: mesh 0 deu o tigre de lava, 7 o de cristal roxo, 12 o de
// gelo, e 15 (que seria o tg010116, inexistente) deixou a montaria invisível —
// valor inválido não derruba o cliente, só não desenha.
//
// Então uma montaria nova são DUAS linhas do ItemList.bin:
//
//   a linha do ITEM      (3995) — o que o jogador carrega: nome, preço, pos
//                                 16384 para o slot 14, EF_CLASS 255
//   a linha da DEFINIÇÃO (1950) — o que o cliente lê: família e variante.
//                                 É clone de uma montaria que já funciona, com
//                                 o mesh trocado.
//
// O servidor não precisa de nada: canEquipSlot libera índice ausente do
// catálogo (item.go:1972), e VisualItemCode manda o índice cru no slot 14.
//
//   node slot.js criar --item 3995 --modelo 334 --pele 12 --nome Tigre_de_Gelo \
//        --icone <índice> --cliente "<pasta>"
//   node slot.js ver --item 3995 --cliente "<pasta>"
//
// As vagas com linha de definição VAZIA no nosso catálogo são oito:
//   3995 → 1950   e   2969-2975 → 924-930
// 3996-3998 cairiam em 1951-1953 (o conjunto Pele de Animal) e o resto da
// faixa 2962-2999 cai em armas que existem. Mais que oito exige emendar a
// cadeia num DLL.

'use strict';

const fs = require('fs');
const path = require('path');

const REG = 140;
const XOR = 0x5a;
const DESLOCAMENTO = 2045; // a regra padrão da cadeia do cliente

function arg(nome, padrao) {
  const i = process.argv.indexOf(nome);
  return i >= 0 ? process.argv[i + 1] : padrao;
}

function carimbo() {
  const d = new Date();
  return `${d.getFullYear()}${String(d.getMonth() + 1).padStart(2, '0')}${String(d.getDate()).padStart(2, '0')}`;
}

function abrir(caminho) {
  const cru = fs.readFileSync(caminho);
  return {
    cru,
    linha(i) {
      const r = Buffer.alloc(REG);
      for (let j = 0; j < REG; j++) r[j] = cru[i * REG + j] ^ XOR;
      return r;
    },
    gravar(i, r) {
      for (let j = 0; j < REG; j++) cru[i * REG + j] = r[j] ^ XOR;
    },
  };
}

function nome(r) {
  let s = '';
  for (let j = 0; j < 64 && r[j]; j++) s += String.fromCharCode(r[j]);
  return s;
}

function efeitos(r) {
  const out = [];
  for (let s = 0; s < 12; s++) {
    const o = 80 + s * 4;
    const e = r.readInt16LE(o);
    const v = r.readInt16LE(o + 2);
    if (e || v) out.push(`${e}:${v}`);
  }
  return out.join(' ');
}

function descrever(cat, i, rotulo) {
  const r = cat.linha(i);
  console.log(`  ${rotulo} ${String(i).padStart(4)}  ${(nome(r) || '(vazia)').padEnd(22)} mesh=${String(r.readInt16LE(64)).padStart(3)} tex=${r.readInt16LE(66)} pos=${r.readUInt16LE(134)} ef=[${efeitos(r)}]`);
}

function main() {
  const cliente = arg('--cliente');
  const cmd = process.argv[2];
  if (!cliente || !['criar', 'ver', 'pele'].includes(cmd)) {
    console.error('uso: node slot.js criar|pele|ver --item <n> [--pele <n>] [--nome <texto>] --cliente "<pasta>"');
    process.exit(1);
  }
  const caminho = path.join(cliente, 'ItemList.bin');
  const cat = abrir(caminho);
  const item = Number(arg('--item', 3995));
  const definicao = item - DESLOCAMENTO;

  if (cmd === 'ver') {
    console.log(`item ${item} → linha de definição ${definicao} (${item} − ${DESLOCAMENTO})`);
    descrever(cat, item, 'item     ');
    descrever(cat, definicao, 'definição');
    return;
  }

  // Trocar a pele de um slot que já existe, sem recriá-lo. A família fica como
  // está — ela vem do EF_CLASS da linha de definição, e essa não se mexe aqui.
  if (cmd === 'pele') {
    const pele = Number(arg('--pele'));
    if (Number.isNaN(pele)) throw new Error('pele precisa de --pele');
    const def = cat.linha(definicao);
    if (!nome(def)) throw new Error(`a linha de definição ${definicao} está vazia — use "criar" primeiro`);
    const antes = def.readInt16LE(64);
    def.writeInt16LE(pele, 64);
    cat.gravar(definicao, def);
    const novoNome = arg('--nome');
    if (novoNome) {
      const it = cat.linha(item);
      it.fill(0, 0, 64);
      it.write(novoNome.slice(0, 63), 0, 'latin1');
      cat.gravar(item, it);
    }
    const copia = caminho + `.antes-slot-${carimbo()}`;
    if (!fs.existsSync(copia)) fs.copyFileSync(caminho, copia);
    fs.writeFileSync(caminho, cat.cru);
    console.log(`item ${item} (linha ${definicao}): pele ${antes} → ${pele}` + (novoNome ? `, nome → ${novoNome}` : ''));
    return;
  }

  const modelo = Number(arg('--modelo'));
  const pele = Number(arg('--pele'));
  const nomeNovo = arg('--nome');
  const icone = arg('--icone') ? Number(arg('--icone')) : null;
  if (!modelo || Number.isNaN(pele) || !nomeNovo) {
    throw new Error('criar precisa de --modelo, --pele e --nome');
  }
  const naFaixa = (item >= 3980 && item <= 3998) || (item >= 2962 && item <= 2999);
  if (!naFaixa) {
    throw new Error(`o cliente aceita montaria em 2360–2389, 2960–2999 e 3980–3998, e só as faixas` +
      ` sem caso nomeado servem de vaga nova: 2962–2999 e 3995–3998. ${item} está fora.`);
  }
  if (modelo < 315 || modelo > 346) {
    throw new Error(`--modelo tem de ser uma das 32 linhas de montaria (315–346), não ${modelo}`);
  }

  // A linha de definição não pode atropelar um item que existe.
  const alvoDef = cat.linha(definicao);
  if (nome(alvoDef)) {
    throw new Error(`a linha de definição ${definicao} é o item "${nome(alvoDef)}" — escolha outro --item`);
  }
  const alvoItem = cat.linha(item);
  if (nome(alvoItem)) {
    throw new Error(`o índice ${item} já é o item "${nome(alvoItem)}"`);
  }

  // A definição é um clone da montaria que já funciona, com a pele trocada:
  // assim família, efeitos e todo o resto vêm de algo comprovado.
  const base = cat.linha(modelo);
  const def = Buffer.from(base);
  def.writeInt16LE(pele, 64);
  cat.gravar(definicao, def);

  // O item que o jogador carrega é um clone do Tigre de Fogo da loja (3990),
  // que é a forma conhecida de uma montaria "especial": pos 16384, EF_CLASS
  // 255, EF_NOTRADE. Só o nome muda.
  const modeloItem = cat.linha(3990);
  const novo = Buffer.from(modeloItem);
  novo.fill(0, 0, 64);
  novo.write(nomeNovo.slice(0, 63), 0, 'latin1');
  cat.gravar(item, novo);

  const copia = caminho + `.antes-slot-${carimbo()}`;
  if (!fs.existsSync(copia)) fs.copyFileSync(caminho, copia);
  fs.writeFileSync(caminho, cat.cru);

  if (icone !== null) {
    const ic = path.join(cliente, 'itemicon.bin');
    const b = fs.readFileSync(ic);
    const copiaIc = ic + `.antes-slot-${carimbo()}`;
    if (!fs.existsSync(copiaIc)) fs.copyFileSync(ic, copiaIc);
    // O itemicon.bin é 1-BASED: o cliente faz valor − 1 antes de indexar a
    // tabela [ItemIcon] (0x40D6D5). --icone recebe a CÉLULA, e gravamos +1.
    b.writeInt32LE(icone + 1, item * 4);
    fs.writeFileSync(ic, b);
    console.log(`itemicon.bin: item ${item} → célula ${icone} (valor ${icone + 1})`);
  }

  console.log(`\nmontaria nova pronta:`);
  descrever(cat, item, 'item     ');
  descrever(cat, definicao, 'definição');
  console.log(`\nno jogo:  /gm item ${item} 106 7`);
  console.log(`cópias em ${path.basename(copia)}`);
}

main();

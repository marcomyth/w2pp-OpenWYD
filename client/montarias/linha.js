#!/usr/bin/env node
// Lê e escreve as linhas de montaria do ItemList.bin do cliente.
//
// COMO O CLIENTE ESCOLHE A MONTARIA (desmontado em 0x50C1AA–0x50C368)
//
// Ele pega `Equip[14] & 0xFFF` e só aceita três faixas — fora delas não
// desenha montaria nenhuma:
//
//     2360–2389   as crias (o nibble alto do código leva o nível, 0 a 13)
//     2960–2999   Klazedale e Gullfaxi
//     3980–3998   as montarias "especiais"
//
// Uma cadeia de comparações em código traduz esse índice num ÍNDICE
// SUBSTITUTO na faixa 315–346 — as linhas "Porco, Javali, …, Tigre de Fogo,
// Grifo, Shire, Helriohdon" do catálogo:
//
//     padrão       substituto = índice − 2045
//     2378 → 333 · 2387-2388 → 336 · 2389 → 346
//     3980-3982 → −3638 · 3983-3985 → −3641 · 3986-3988 → −3644 · 3989 → 345
//     3990 → 334 · 3991 → 335 · 3992 → 318 · 3993-3994 → 200
//     2960-2961 → −2616
//
// Com o substituto na mão ele lê `ItemList[substituto]` nos campos +64 (mesh) e
// +66 (texture) e guarda em +0x126 / +0x128 da entidade; +0x124 fica com
// `substituto − 315`.
//
// O QUE AINDA NÃO SE SABE
//
// Se o modelo montado sai do +64/+66 dessa linha ou do próprio índice. Os
// valores lá são pequenos (0 a 10) e não batem com a família do bicho — Lobo,
// Cavalo, Tigre e Grifo têm todos mesh 0 —, o que sugere índice. Só o jogo
// responde: mude o mesh de uma linha e veja se a montaria troca.
//
//   node linha.js listar               --cliente "<pasta>"
//   node linha.js pôr <linha> <mesh> <tex> --cliente "<pasta>"

'use strict';

const fs = require('fs');
const path = require('path');

const REG = 140;
const XOR = 0x5a;
const PRIMEIRA = 315;
const ULTIMA = 346;

function arg(nome, padrao) {
  const i = process.argv.indexOf(nome);
  return i >= 0 ? process.argv[i + 1] : padrao;
}

function carimbo() {
  const d = new Date();
  return `${d.getFullYear()}${String(d.getMonth() + 1).padStart(2, '0')}${String(d.getDate()).padStart(2, '0')}`;
}

// O catálogo é ofuscado com um XOR fixo de 0x5A, sem cabeçalho.
function abrir(caminho) {
  const cru = fs.readFileSync(caminho);
  return {
    cru,
    linha(i) {
      const r = Buffer.alloc(REG);
      for (let j = 0; j < REG; j++) r[j] = cru[i * REG + j] ^ XOR;
      return r;
    },
    gravarLinha(i, r) {
      for (let j = 0; j < REG; j++) cru[i * REG + j] = r[j] ^ XOR;
    },
  };
}

function nome(r) {
  let s = '';
  for (let j = 0; j < 64 && r[j]; j++) s += String.fromCharCode(r[j]);
  return s;
}

// Quais índices de item caem em cada linha, pela cadeia do cliente.
function quemAponta(substituto) {
  const fixos = { 333: [2378], 336: [2387, 2388], 346: [2389], 334: [3990], 335: [3991], 318: [3992] };
  const fora = new Set([2378, 2387, 2388, 2389]);
  const lista = [...(fixos[substituto] || [])];
  const padrao = substituto + 2045;
  if (padrao >= 2360 && padrao <= 2389 && !fora.has(padrao)) lista.push(padrao);
  for (const [lo, hi, off] of [[3980, 3982, 3638], [3983, 3985, 3641], [3986, 3988, 3644]]) {
    for (let i = lo; i <= hi; i++) if (i - off === substituto) lista.push(i);
  }
  if (substituto === 345) lista.push(3989);
  for (const i of [2960, 2961]) if (i - 2616 === substituto) lista.push(i);
  return lista.sort((a, b) => a - b);
}

function main() {
  const cliente = arg('--cliente');
  const cmd = process.argv[2];
  if (!cliente || !['listar', 'pôr', 'por'].includes(cmd)) {
    console.error('uso: node linha.js listar | pôr <linha> <mesh> <tex> --cliente "<pasta>"');
    process.exit(1);
  }
  const caminho = path.join(cliente, 'ItemList.bin');
  const cat = abrir(caminho);

  if (cmd === 'listar') {
    console.log('linha | nome                        mesh  tex | itens que caem nela');
    for (let i = PRIMEIRA; i <= ULTIMA; i++) {
      const r = cat.linha(i);
      const aponta = quemAponta(i);
      console.log(
        String(i).padStart(5) + ' | ' + nome(r).padEnd(26) +
        String(r.readInt16LE(64)).padStart(5) + String(r.readInt16LE(66)).padStart(5) +
        ' | ' + (aponta.length ? aponta.join(', ') : '—'));
    }
    return;
  }

  const [alvo, mesh, tex] = process.argv.slice(3).filter((a) => /^-?\d+$/.test(a)).map(Number);
  if (alvo < PRIMEIRA || alvo > ULTIMA) throw new Error(`a linha ${alvo} está fora de ${PRIMEIRA}–${ULTIMA}: o cliente não a usa como montaria`);
  const r = cat.linha(alvo);
  const antes = [r.readInt16LE(64), r.readInt16LE(66)];
  r.writeInt16LE(mesh, 64);
  r.writeInt16LE(tex, 66);
  cat.gravarLinha(alvo, r);
  const copia = caminho + `.antes-montaria-${carimbo()}`;
  if (!fs.existsSync(copia)) fs.copyFileSync(caminho, copia);
  fs.writeFileSync(caminho, cat.cru);
  console.log(`linha ${alvo} (${nome(r)}): mesh ${antes[0]}→${mesh}, tex ${antes[1]}→${tex}`);
  console.log(`itens afetados: ${quemAponta(alvo).join(', ') || '—'}`);
  console.log(`cópia em ${path.basename(copia)}`);
}

main();

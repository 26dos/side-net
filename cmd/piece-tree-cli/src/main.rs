\
use anyhow::{Context, Result};
use serde::Serialize;
use std::io::{Read, stdin};
use std::cmp::min;

use storage_proofs_core::fr32::bytes_into_fr;
use neptune::poseidon::Poseidon;
use neptune::Strength;
use blstrs::Scalar as Fr;

// Filecoin-compatible Poseidon Merkle (arity=8) with Fr32 leaves.
const ARITY: usize = 8;
const WINDOW_BYTES: usize = 1 << 20; // 1MiB
const FR_LEAF: usize = 32;

#[derive(Serialize)]
struct WindowPath {
    window_id: u64,
    start_leaf: u64,
    leaf_count: u64,
    siblings: Vec<Vec<String>> // per-level siblings (hex 32B)
}

#[derive(Serialize)]
struct Output {
    hash_algo: String,           // "poseidon-filecoin"
    arity: usize,                // 8
    leaf_size: usize,            // 32
    total_leaves: u64,
    root: String,                // hex 32B
    window_size_bytes: usize,    // 1MiB
    window_paths: Vec<WindowPath>
}

fn poseidon_hash(children: &[Fr]) -> Fr {
    let mut p = Poseidon::new_with_strength(children.len(), Strength::Standard).expect("poseidon");
    for c in children { p.input(*c).expect("input"); }
    p.hash().expect("hash")
}

fn pad_to_power(mut n: usize, arity: usize) -> usize {
    if n <= arity { return arity; }
    let mut t = arity;
    while t < n { t *= arity; }
    t
}

struct BuiltTree { levels: Vec<Vec<Fr>> } // levels[0]=leaves, last=len=1

fn build_tree(mut leaves: Vec<Fr>, arity: usize) -> BuiltTree {
    if leaves.is_empty() { leaves.push(Fr::zero()); }
    let target = pad_to_power(leaves.len(), arity);
    while leaves.len() < target { leaves.push(Fr::zero()); }

    let mut levels: Vec<Vec<Fr>> = vec![leaves];
    while levels.last().unwrap().len() > 1 {
        let prev = levels.last().unwrap();
        let mut parent = Vec::with_capacity((prev.len() + arity - 1)/arity);
        for chunk in prev.chunks(arity) {
            let mut buf = vec![Fr::zero(); arity];
            for i in 0..arity { buf[i] = if i < chunk.len() { chunk[i] } else { Fr::zero() }; }
            parent.push(poseidon_hash(&buf));
        }
        levels.push(parent);
    }
    BuiltTree { levels }
}

// Collect siblings for the group containing start_leaf across levels to the root.
fn window_path(levels: &Vec<Vec<Fr>>, arity: usize, start_leaf: usize) -> Vec<Vec<Fr>> {
    let mut idx = start_leaf;
    let mut sib_layers: Vec<Vec<Fr>> = Vec::new();
    for level in 0..(levels.len()-1) {
        let group_size = arity.pow(level as u32);
        let group_base = (idx / group_size / arity) * arity;
        let my_pos = (idx / group_size) % arity;

        let mut sibs = Vec::new();
        for a in 0..arity {
            if a == my_pos { continue; }
            let repr_index = (group_base + a) * group_size;
            let node = if repr_index < levels[level].len() { levels[level][repr_index] } else { Fr::zero() };
            sibs.push(node);
        }
        sib_layers.push(sibs);
        idx = (idx / group_size) / arity;
    }
    sib_layers
}

fn main() -> Result<()> {
    // Read raw bytes (piece bytes). For production-scale pieces, stream via temp file.
    let mut raw = Vec::new();
    stdin().lock().read_to_end(&mut raw).context("read stdin")?;

    // Pad to 32-byte chunks (Fr32 chunking). For full Filecoin Fr32 bit-level mapping,
    // upstream pipelines apply bit-padding; here we map each 32B chunk into Fr.
    if raw.is_empty() { raw.resize(FR_LEAF, 0); }
    if raw.len() % FR_LEAF != 0 {
        let pad = FR_LEAF - (raw.len() % FR_LEAF);
        raw.extend(std::iter::repeat(0u8).take(pad));
    }

    // Map to Fr leaves
    let mut leaves: Vec<Fr> = Vec::with_capacity(raw.len()/FR_LEAF);
    for chunk in raw.chunks(FR_LEAF) {
        let mut b = [0u8; 32];
        b.copy_from_slice(chunk);
        let fr = bytes_into_fr(&b).context("fr32 map failed")?;
        leaves.push(fr);
    }

    // Build tree
    let tree = build_tree(leaves, ARITY);
    let root = *tree.levels.last().unwrap().first().unwrap();
    let mut root_bytes = [0u8;32];
    let mut cur = &mut root_bytes[..];
    root.serialize(&mut cur).expect("fr->bytes");
    let root_hex = hex::encode(root_bytes);

    // Export window paths
    let total_leaves = tree.levels.first().unwrap().len();
    let leaves_per_window = (WINDOW_BYTES + FR_LEAF - 1) / FR_LEAF;

    let mut paths: Vec<WindowPath> = Vec::new();
    let mut start = 0usize;
    let mut wid = 0u64;
    while start < total_leaves {
        let cnt = std::cmp::min(leaves_per_window, total_leaves - start);
        let sib_layers = window_path(&tree.levels, ARITY, start);
        let mut out_layers: Vec<Vec<String>> = Vec::with_capacity(sib_layers.len());
        for layer in sib_layers {
            let mut one = Vec::with_capacity(layer.len());
            for fr in layer {
                let mut bs=[0u8;32];
                let mut c = &mut bs[..];
                fr.serialize(&mut c).unwrap();
                one.push(hex::encode(bs));
            }
            out_layers.push(one);
        }
        paths.push(WindowPath{ window_id: wid, start_leaf: start as u64, leaf_count: cnt as u64, siblings: out_layers });
        start += cnt;
        wid += 1;
    }

    let out = Output{
        hash_algo: "poseidon-filecoin".to_string(),
        arity: ARITY,
        leaf_size: FR_LEAF,
        total_leaves: total_leaves as u64,
        root: root_hex,
        window_size_bytes: WINDOW_BYTES,
        window_paths: paths
    };
    println!("{}", serde_json::to_string(&out)?);
    Ok(())
}

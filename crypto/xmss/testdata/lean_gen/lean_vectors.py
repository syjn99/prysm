"""Generate extra leanXMSS vectors with the compiled Lean model (EthLib Exports.Xmss).

Usage: python3 -I lean_vectors.py <gen binary> <out.json>
"""
import concurrent.futures as cf
import json
import subprocess
import sys
import time

gen, out_path = sys.argv[1], sys.argv[2]


def run(*args):
    t = time.time()
    r = subprocess.run([gen, *args], capture_output=True, text=True, check=True)
    print(f"{args[0]} done in {time.time() - t:.0f}s", file=sys.stderr, flush=True)
    return r.stdout.strip()


def hx(b):
    return bytes(b).hex()


seeds = {
    "known_answer": hx((37 * i + 11) % 256 for i in range(32)),
    "zero": hx([0] * 32),
    "counting": hx(range(32)),
}
messages = {
    "known_answer": hx((7 * i + 3) % 256 for i in range(32)),
    "zero": hx([0xFF] * 32),
    "counting": hx((13 * i + 5) % 256 for i in range(32)),
}
indices = [0, 1, 65535, 65536, 2**32 - 1]

with cf.ThreadPoolExecutor(max_workers=18) as ex:
    pk_f = {name: ex.submit(run, "keygen", seed) for name, seed in seeds.items()}
    sig_f = {
        (name, idx): ex.submit(run, "sign", seed, str(idx), messages[name])
        for name, seed in seeds.items()
        for idx in indices
    }
    pks = {name: f.result() for name, f in pk_f.items()}
    sigs = {k: f.result() for k, f in sig_f.items()}

key_gen = [{"seed": seeds[n], "public_key": pks[n]} for n in seeds]
sign = []
for (name, idx), sig in sigs.items():
    sign.append({"seed": seeds[name], "leaf_index": idx, "message": messages[name], "signature": sig})

# Each tamper flips the low bit of one byte of one input of a signed case.
LAYER = 1200
tampers = [("none", None, 0)]
tampers += [
    ("signature", 0, 1),                      # leaf index
    ("signature", 4, 1),                      # bottom randomizer
    ("signature", 4 + 16, 1),                 # bottom chain value 0
    ("signature", 4 + 16 + 57 * 16, 1),       # bottom chain value 57
    ("signature", 4 + 16 + 58 * 16, 1),       # bottom path, level 0
    ("signature", 4 + LAYER - 1, 1),          # bottom path, level 15
    ("signature", 4 + LAYER, 1),              # top randomizer
    ("signature", 4 + LAYER + 16, 1),         # top chain value 0
    ("signature", 4 + LAYER + 16 + 58 * 16, 1),  # top path, level 0
    ("signature", 2 * LAYER + 3, 1),          # top path, level 15
    ("message", 0, 1),
    ("message", 31, 0x80),
    ("public_key", 0, 1),                     # root
    ("public_key", 16, 1),                    # public parameter
]


def apply(case, target, offset, xor):
    vals = {"public_key": case["pk"], "message": case["message"], "signature": case["signature"]}
    if target != "none":
        b = bytearray(bytes.fromhex(vals[target]))
        b[offset] ^= xor
        vals[target] = b.hex()
    return vals


def case_of(i):
    s = sign[i]
    name = next(n for n in seeds if seeds[n] == s["seed"])
    return {"pk": pks[name], "message": s["message"], "signature": s["signature"]}


# Every signature verifies untampered. Each tamper is applied to one signature, tamper j to
# sign case j mod len(sign), so the negatives cover different seeds and leaf indices.
jobs = [(i, "none", None, 0, apply(case_of(i), "none", None, 0)) for i in range(len(sign))]
for j, (target, offset, xor) in enumerate(tampers[1:]):
    i = j % len(sign)
    jobs.append((i, target, offset, xor, apply(case_of(i), target, offset, xor)))

verify = []
with cf.ThreadPoolExecutor(max_workers=16) as ex:
    futs = [ex.submit(run, "verify", v["public_key"], v["message"], v["signature"]) for *_, v in jobs]
    for (i, target, offset, xor, _), f in zip(jobs, futs):
        r = f.result()
        assert r in ("0", "1"), r
        entry = {"sign_case": i, "target": target, "valid": r == "1"}
        if target != "none":
            entry["offset"] = offset
            entry["xor"] = xor
        verify.append(entry)

doc = {
    "source": "ethereum/EthLib 6390c73ef040311e807b9047d99dca6b1be1c42f, "
    "EthLib.Cryptography.Exports.Xmss (keyGenExport, signExport, verifyExport), "
    "compiled with Lean v4.35.0",
    "key_gen": key_gen,
    "sign": sign,
    "verify": verify,
}
with open(out_path, "w") as f:
    json.dump(doc, f, indent=1)
    f.write("\n")

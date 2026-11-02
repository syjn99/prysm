import EthLib.Cryptography.Exports.Xmss

open EthLib.Cryptography.Xmss.Exports

def nib (c : Char) : UInt8 :=
  if c.isDigit then UInt8.ofNat (c.toNat - 48)
  else if 'a' ≤ c ∧ c ≤ 'f' then UInt8.ofNat (c.toNat - 87)
  else UInt8.ofNat (c.toNat - 55)

def hexToBytes (s : String) : ByteArray := Id.run do
  let cs := s.toList.toArray
  let mut out := ByteArray.empty
  for i in [0:cs.size / 2] do
    out := out.push (nib cs[2 * i]! * 16 + nib cs[2 * i + 1]!)
  return out

def toHex (b : ByteArray) : String :=
  let h := "0123456789abcdef".toList.toArray
  String.join (b.toList.map fun x => String.mk [h[x.toNat / 16]!, h[x.toNat % 16]!])

def main (args : List String) : IO UInt32 := do
  match args with
  | ["keygen", seed] =>
    IO.println (toHex (← keyGenExport (hexToBytes seed)))
    return 0
  | ["sign", seed, idx, msg] =>
    IO.println (toHex (← signExport (hexToBytes seed) (UInt32.ofNat idx.toNat!) (hexToBytes msg)))
    return 0
  | ["verify", pk, msg, sig] =>
    IO.println (toString (← verifyExport (hexToBytes pk) (hexToBytes msg) (hexToBytes sig)))
    return 0
  | _ =>
    IO.eprintln "usage: gen keygen SEED | gen sign SEED IDX MSG | gen verify PK MSG SIG"
    return 2

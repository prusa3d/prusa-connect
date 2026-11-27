// What the live socket carries, read only as far as a decoder needs: whether a
// picture is a keyframe, and the codec string WebCodecs is configured with.
// The server frames each picture as Annex B behind one flag byte, so the
// picture itself is handed to the decoder as it arrived.

/** One picture off the live socket. */
export interface SocketPicture {
  readonly key: boolean;
  /** The access unit, Annex-B framed. */
  readonly unit: Uint8Array;
}

/** Splits one socket message into its flag and its picture. */
export const pictureOf = (message: ArrayBuffer): SocketPicture | undefined => {
  const bytes = new Uint8Array(message);
  if (bytes.length < 2) return undefined;

  return { key: bytes[0] === 1, unit: bytes.subarray(1) };
};

const SPS = 7;
const TYPE_MASK = 0x1f;

const hex = (byte: number): string => byte.toString(16).padStart(2, '0');

/**
 * The codec string of the stream, `avc1.` and the SPS's profile, constraint
 * flags and level, as WebCodecs takes it. Undefined for a picture that carries
 * no SPS, which no decoder can be configured from.
 */
export const codecOf = (unit: Uint8Array): string | undefined => {
  for (let at = 0; at + 3 < unit.length; at += 1) {
    // A start code is 00 00 01, and the four-byte form ends the same way.
    if (unit[at] !== 0 || unit[at + 1] !== 0 || unit[at + 2] !== 1) continue;
    const header = at + 3;
    if (((unit[header] ?? 0) & TYPE_MASK) !== SPS) continue;
    if (header + 3 >= unit.length) return undefined;

    return `avc1.${hex(unit[header + 1] ?? 0)}${hex(unit[header + 2] ?? 0)}${hex(unit[header + 3] ?? 0)}`;
  }

  return undefined;
};

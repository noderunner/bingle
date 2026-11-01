# 0074. The second factor's QR code is drawn by a package of our own

Status: accepted (2026-10-31)

## Context

Setting up a second factor ([ADR 0069](0069-second-factor-and-lockout.md)) gave a key and an `otpauth://`
link, and nothing a phone's camera can read — which is how most people add an account to Google
Authenticator and the like. Bingle has eleven direct dependencies and treats the number as a feature,
and the static JavaScript has no build step, so neither a Go module nor an npm package comes free.

## Decision

- **`internal/qr`** encodes bytes (byte mode) at error correction level M in versions 1 to 10 — up to
  213 bytes, where a setup link with the longest username is 139 — and draws an SVG. That is all of
  the standard bingle needs; numeric, alphanumeric and kanji modes, other levels and larger versions
  are left out. About 450 lines with comments.
- `POST /auth/totp/setup` answers `qr_svg` beside `secret` and `uri`. The page shows it as an
  `<img>` with a `data:` URL, never as markup, so nothing in it could run; it holds nothing the
  answer does not already hold.
- It is tested against the standard's worked example for the error correction, its tables for the
  format and version information, the data-module count of every version, unbroken timing patterns,
  and a fingerprint of a setup link's symbol that a real reader (OpenCV's) decoded when it was
  written — as was every version with every mask. The fingerprint changes only if the drawing does;
  a change has to be read back by a real reader before the fingerprint is updated.

## Consequences

- No new dependency, and no QR code made by a third party from the secret.
- A bug in the drawing would be ours to find: the fingerprint test pins the output, and the key and
  the link are still shown beside the code for typing in.
- Anything else wanting a QR code (a guest key's link to another device, say) can use the package if
  it fits in 213 bytes.

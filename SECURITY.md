# Security Policy

## Supported versions

This is a v1 library at an early stage. There is **no formal versioning or
support guarantee yet**: the only supported version is the current `main`
branch and the most recent tag. Releases before `v1.0.0` may change exported
APIs and event semantics without a deprecation period. Fixes are applied to
`main`; there are no maintained release branches and no backports.

## Audit status

**This code is unaudited.** It has not undergone any external security review.
Consumers deriving financial or analytical data from its output should
validate results independently before relying on them.

## In scope

Reports about this package's own behaviour, specifically:

- **Panics on malformed or adversarial ledger data.** A `Transformer` must
  never panic on unexpected input — it returns an error. Any input, whether a
  crafted XDR fixture or a real ledger, that makes `classic.Transformer`,
  `soroban.Transformer`, a `ProtocolAdapter`, or a `Source` panic, deadlock,
  or exhaust memory is a valid report.
- **Incorrect `TradeEvent` derivation that could mislead a downstream
  consumer.** Wrong amount, wrong asset, wrong price, wrong trade direction
  (base and counter reversed), wrong account attribution, duplicated events,
  or silently dropped trades that should have been emitted.
- **Integer overflow or precision loss in amount parsing.** Incorrect handling
  of `int64` stroop amounts or `i128` Soroban amounts, including overflow,
  truncation, sign errors, or division by zero in price computation.
- Incorrect decoding of contract events that lets a contract impersonate a
  registered protocol, for example a non-router contract's events being
  attributed to Soroswap.

## Out of scope

- The Stellar network, Stellar Core, Stellar RPC, and the
  `github.com/stellar/go-stellar-sdk` dependency. Report those to their
  respective maintainers.
- The Soroswap contracts themselves, or any other on-chain protocol. This
  package only reads their emitted events.
- Anything in a consumer's own `Sink` implementation, including how events are
  stored, transmitted, or exposed.
- Operational concerns of whichever RPC endpoint you point the processor at,
  including endpoints serving incomplete or malicious ledger data.
- Denial of service caused solely by supplying an unbounded ledger range.

## Reporting a vulnerability

**Please do not open a public issue for a security report.**

Report privately through GitHub: go to the repository's **Security** tab and
choose **Report a vulnerability** to open a private security advisory.

Helpful details: the affected version or commit, a minimal reproduction
(a failing test or an XDR fixture is ideal), the ledger sequence and network
if the input came from a live network, and the impact you observed.

## Response timeline

This is a young, solo-maintained project. Responses are **best effort, with
no service level agreement**. Expect an acknowledgement within a couple of
weeks; a fix timeline depends on severity and maintainer availability. If a
report goes unanswered and you need to move forward, you are free to disclose
it publicly at your discretion.

Fixes are released as a new tag, and the advisory is published once the fix is
available. Reporters are credited unless they ask otherwise.

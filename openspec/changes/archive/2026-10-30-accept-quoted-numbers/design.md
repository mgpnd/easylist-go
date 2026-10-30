# Design

## Context

See proposal.md for motivation. Every PostgreSQL driver lowers predicates through easylist-sqlpg, and every scalar value, whether a single literal, a list item, a `BETWEEN` bound, or an array item, ends in one conversion step that switches on the field type and the literal kind. For integer, float, and numeric that step accepts only the number kind. The parser already gives the decoded string text, and the planner passes plain literals through untouched.

The quoted `BETWEEN 'a,b'` form and quoted array lists already reach the same step by relabelling their split pieces as numbers before converting them. So the conversion rules for number text already exist; only the kind check blocks a quoted value.

## Goals / Non-Goals

**Goals:**
- A quoted number on a number field goes through exactly the code path an unquoted one does, so the two can never drift apart.
- Error texts stay byte-identical for every input that still fails. The conformance suite compares each driver's error with easylist-sqlpg's.

**Non-Goals:**
- Coercing quoted values for boolean, or numbers for text-like types.
- Changing the parser or the AST: `'30'` stays a string literal, and the planner stays unaware of plain literal values.
- Trimming spaces around a quoted value.

## Decisions

**Relax the kind check in the shared conversion step, for the three number types only.** The integer, float, and numeric cases accept the string kind next to the number kind, then run the existing parse and pattern checks unchanged.
- *Alternative: rewrite string literals to numbers in the planner.* Rejected because the planner would need per-type literal knowledge it deliberately does not have, and the plan would no longer show what the user wrote.
- *Alternative: let the parser emit a number for `'30'`.* Rejected because the parser has no schema, and `name = '30'` on a text field must stay a string.
- *Alternative: accept the string kind for every type.* Rejected for now because boolean would need its own spelling rules, and nothing needs it yet.

**No trimming.** An unquoted number can never contain spaces, so accepting `' 30'` would make the quoted form looser than the unquoted one. The comma-split forms keep trimming each piece, as they do today. Integers keep the 64-bit parse, so `'30.0'` fails exactly as `30.0` does. Float and numeric keep the existing number pattern, so `'1e3'`, `'.5'`, and `'NaN'` fail.

**The quoted `BETWEEN 'a,b'` and quoted array paths stay as they are.** They already relabel each piece before converting it. With the relaxed check the relabelling becomes redundant for number types, but keeping it avoids touching code whose behavior does not change.

## Risks / Trade-offs

- [A client that relied on `age = '30'` failing, for example to detect a type mismatch] → Unlikely, since the error was the only effect. The proposal states that previously rejected filters now compile.
- [Third-party drivers outside this repository that implement the contract themselves keep rejecting quoted numbers] → The contract change is called out in the proposal; the conformance suite will fail for them until they follow it.
- [Quoted numbers look like a precedent for coercing every type] → The spec scenario for `active = 'true'` pins booleans as unchanged, so widening needs its own change.

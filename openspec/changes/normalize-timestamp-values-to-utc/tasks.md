# Tasks

## 1. easylist-core

- [x] 1.1 In `Runtime.Stream` (`easylist-core/runtime.go`), after `holderValue` and before the array branch, convert the value of a non-array `Timestamp` field to UTC with `UTC()`. Leave `Date` fields and calculated fields untouched. Verify with 1.3.
- [x] 1.2 In the `Timestamp` case of `decodeArray` (`easylist-core/array.go`), return each parsed element converted with `UTC()`. Update the comment above `timestampLayouts` so it says that elements are returned in UTC and that an element without an offset is read as UTC. Verify with 1.3.
- [x] 1.3 Tests in `easylist-core`:
  - In `TestStreamScansTypedValues` (`runtime_test.go`), script `at` as the same instant in `time.FixedZone("", 3*3600)`. Script `day` as midnight 2026-10-25 in that zone. Assert that the `at` row value has location `time.UTC` and equals the instant, and that `day` still formats as `2026-10-25`.
  - Add a case with a calculated timestamp field that returns a `+03:00` time, and assert that its location is kept.
  - In `TestDecodeArrayElements` (`array_test.go`), expect the `+02:00` element as `2026-10-25T08:00:00Z` in UTC.
  - Make the comparison check `Location() == time.UTC` for every timestamp element, not just `Equal`.
  - Verify with `go test -race ./...` in `easylist-core`.
- [x] 1.4 In `easylist-core/README.md`, say in the types table or right under it that timestamp values read from the database are returned in UTC whatever zone the database handle scans them in, and that dates and calculated values are not converted. In the arrays paragraph, change "read as UTC when there is no offset" to say that every timestamp element is returned in UTC. Add one sentence telling callers that want local times to call `In(loc)`. Verify by reading both sections against the "Typed row values" and "Array row values" requirements in the change's `core-list-execution` delta.

## 2. easylist-gqlgen

- [x] 2.1 In `easylist-gqlgen/internal/fixture/graph/graph_test.go`, script `created_at` for at least one sample row as `created.In(time.FixedZone("", 3*3600))`, keeping the expected `"created_at":"2026-10-25T10:00:00Z"` in `TestValuesAndCalculatedFields`. If `TestArrayValues` has a timestamp array, give one of its elements a non-UTC offset in the scripted JSON as well, and expect a `Z` value. Verify with `go test -race ./...` in `easylist-gqlgen`. The test fails before group 1 and passes after it.

## 3. easylist-behavior

Do this group only once `easylist-behavior` is on the branch being applied (today it lives on `claude/project-behavior-visibility-4d6355`). If it is not there yet, leave the boxes unticked and record in the module's own change that the codec can go.

- [x] 3.1 In `easylist-behavior/harness_test.go` `createFixture`, remove the `stdlib.OptionAfterConnect` hook that registers `pgtype.TimestamptzCodec{ScanLocation: time.UTC}`, and its comment. Remove the `pgtype` import if nothing else uses it. Keep `cfg.RuntimeParams["timezone"] = "UTC"`, and change its comment to say it fixes how the database evaluates zone-dependent SQL. Verify with `go vet ./...` in `easylist-behavior`.
- [x] 3.2 With the compose database up, run the snapshot suite without `-update` under `TZ=UTC`, `TZ=Europe/Moscow`, and `TZ=Asia/Tokyo`: `TZ=… EASYLIST_PG_DSN=postgres://postgres@127.0.0.1:54329/postgres?sslmode=disable go test -race -count=1 ./...` in `easylist-behavior`. Verify that all three runs pass with no golden file changed (`git status` clean under `easylist-behavior/testdata`).

## 4. Workspace check

- [x] 4.1 From the repository root, run the full workspace suite with a live database under a non-UTC zone: `TZ=Asia/Tokyo EASYLIST_PG_DSN=… go test -race $(go list -m -f '{{.Dir}}/...')`. Verify that every module passes, including easylist-export, whose CSV and XLSX timestamp output must be unchanged, and that no live conformance case is skipped.

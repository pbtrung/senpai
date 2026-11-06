# Development checks

Run the unit tests, race detector, and static checks before submitting changes:

```sh
go test -race ./...
go vet ./...
go build ./cmd/senpai
```

For the real bouncer integration test, follow the
[prebuilt soju-rs instructions](soju-rs.md). Without `SOJU_RS_BINARY`, the
integration test is skipped.

The small local varlink service embeds `fr.delthas.senpai.varlink` directly.
Update its typed bindings in `varlinkservice/service.go` if that interface
changes; no code generator is required.

## Dependencies

Versions in `go.mod` include explicit constraints on transitive test and tooling
modules. These keep the full module graph current even where an upstream module
still declares older minimum versions. `go mod tidy` can remove those constraints
and select the older upstream versions again. After updating or tidying, check:

```sh
go list -m -u all
go mod verify
```

Keep dependency upgrades in a separate commit from behavior changes and run the
soju-rs integration test after upgrading.

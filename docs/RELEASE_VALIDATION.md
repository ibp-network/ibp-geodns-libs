# Release Validation

This checklist is for any release of `github.com/ibp-network/ibp-geodns-libs`
that affects shared config, NATS transport, consensus, collator finalize logic,
or any message contract used by `ibp-geodns`, `ibp-geodns-monitor`, and
`ibp-geodns-collator`.

## Pre-release

1. Record the exact SHAs of:
   - `ibp-geodns-libs`
   - `ibp-geodns`
   - `ibp-geodns-monitor`
   - `ibp-geodns-collator`
2. Run `go test ./...` in:
   - `ibp-geodns-libs`
   - `ibp-geodns`
   - `ibp-geodns-monitor`
   - `ibp-geodns-collator`
3. Capture baseline NATS monitoring data from at least two cluster nodes:
   - `/varz`
   - `/routez`
   - `/connz?subs=1`
   - `/subsz?subs=1`

## Dependency bump

After tagging the new `ibp-geodns-libs` version:

1. Bump `go.mod` in:
   - `ibp-geodns`
   - `ibp-geodns-monitor`
   - `ibp-geodns-collator`
2. Re-run `go test ./...` in all three repos.
3. Push the version bumps together.

## Deploy order

1. Deploy `ibp-geodns-monitor`
2. Deploy `ibp-geodns-collator`
3. Deploy `ibp-geodns`

If the release affects request/reply or usage record semantics, deploy DNS and
collator together to avoid mixed payload assumptions.

## Live checks

After deploy, confirm:

1. Monitor logs show expected proposal gating and remote sender information.
2. NATS `subsz` counters increase for:
   - `consensus.propose`
   - `consensus.vote`
   - `consensus.finalize`
3. Monitors do not finalize with only one active monitor when quorum should
   require two or more.
4. Collator logs show proposal, vote, and finalize flow for the same IDs.
5. Route health remains stable and `slow_consumer_stats` does not spike during
   the test window.

## Rollback trigger

Roll back if any of the following happen:

1. `consensus.vote` or `consensus.finalize` traffic flatlines after proposals.
2. Proposal IDs show local finalize before expected quorum is available.
3. NATS route slow-consumer counts rise during rollout.
4. Collator stops caching new proposals or stops recording votes/finalizes.

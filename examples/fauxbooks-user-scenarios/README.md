# Fauxbooks End-User Scenario E2E

This example shows the current end-to-end user workflow for grounded finance answers. It uses [Fauxbooks](https://github.com/humzakhan/fauxbooks), a separate open source project, to generate deterministic synthetic `monthly-close` exports. Provenant then runs Claude Code against those generated files, requires every final numeric answer to be Provenant-grounded, recomputes truth from the CSVs, and writes a report with the evidence paths.

The example is intentionally framed around practical controller-style questions rather than kernel internals:

1. What was Q1 revenue?
2. Why does cash received not match booked revenue?
3. Which customers owe us money, and who has the largest balance?

Run it from the repository root:

```sh
hack/e2e/fauxbooks-user-scenarios.sh \
  --seed 424200 \
  --out /tmp/provenant-fauxbooks-user-scenarios
```

The checked-in report from the latest passing run is [`report.md`](report.md).

Files:

- `plan.md`: scenario definitions, dataset commands, and verifier rules.
- `goal-runner.md`: one-command runner contract and acceptance criteria.
- `golden-report.md`: target shape for the final markdown findings report.
- `report.md`: latest checked-in passing run report.

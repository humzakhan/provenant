# Extract Backend: Azure Document Intelligence

Status: **v0.5 / deferred**

> Azure Document Intelligence (formerly Form Recognizer) is the planned secondary extraction backend, targeting customers on Azure or with Microsoft-leaning IT.

## Why deferred

Textract covers the v0 cases. Azure DI is a parity backend, not a differentiator. Building it before v0 stability would consume bandwidth for parity that customers haven't yet requested in volume.

## Design constraints when we build it

1. **Auth**: Azure Active Directory token or API key.
2. **Models**: Pre-built models for `prebuilt-bankStatement`, `prebuilt-invoice`, `prebuilt-receipt`; custom-trained models for `kv_pairs`.
3. **Schema mapping**: Azure DI's `Document` and `Fields` map to the canonical schema with mostly clean correspondence.
4. **Captured reads**: Same convention as Textract backend.
5. **Confidence calibration**: Per-tool calibration map, recomputed quarterly.

## Parity expectations

The Azure DI backend must produce canonical responses that are *substitutable* with Textract — a skill pack should not need to know which backend is in use. Confidence scores will differ, but calibrated confidence and downstream behavior should be equivalent.

## Known integration friction

- Custom models require training on customer-specific documents. We do not ship training infrastructure in v0.5; customers provide trained model IDs in config.
- Azure DI's free tier has aggressive rate limits; production tenants need paid tier.

## Schedule

v0.5 timeline depends on customer demand from Azure shops. Earliest realistic: 6 months after v0 GA.

## References

- [`tools/extract/README.md`](../README.md)
- Azure Document Intelligence: https://learn.microsoft.com/en-us/azure/ai-services/document-intelligence/

# Provider assumptions verified September 12, 2026

The [official GPT-5.6 Sol model page](https://developers.openai.com/api/docs/models/gpt-5.6-sol) was opened and checked. It lists `gpt-5.6-sol`, Responses, streaming and Structured Outputs; standard text rates are USD 4 per million input tokens and USD 20 per million output tokens. The page lists cache-write pricing at 1.25 times uncached input, and a higher long-context rate above 272,000 input tokens. The planned request-byte cap plus conservative overhead remains below that threshold.

This verifies published configuration/pricing, not account/model access or actual invoiced cost. No API request was made for this check. Exact request fields, effective service-tier policy and conservative reservation implementation must be tested and frozen before live dispatch. Do not change model, limits or tier after freeze to recover a failure.

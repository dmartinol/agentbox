---
layout: docs
title: Provider commands
nav_title: Providers
command: provider
order: 8
subcommands:
  - setup
---

Providers register inference endpoints with OpenShell. Provider configuration
is separate from per-run agent configuration; gateway-mode profiles reference
a provider by name.

## setup
{: #provider-setup }

Register a provider.

```text
agentbox provider setup \
  [--type gateway-bearer|gateway-header|openai|anthropic] \
  [--host HOST] [--credential ENV_NAME] [--gateway NAME] \
  [--dry-run] [--update] NAME
```

| Flag | Default | Description |
| --- | --- | --- |
| `--type` | `gateway-bearer` | `gateway-bearer`, `gateway-header`, `openai`, or `anthropic`. |
| `--host` | — | Gateway host, for the gateway provider types. |
| `--credential` | `AGENTBOX_MODEL_KEY` | Environment-variable **name** holding the API key. |
| `--gateway` | — | Gateway name to register the provider with. |
| `--dry-run` | `false` | Print the planned registration without running it. |
| `--update` | `false` | Update an existing AgentBox provider profile. |

Use `gateway-bearer` for OpenAI-compatible gateway APIs and `gateway-header`
for Anthropic-compatible gateway APIs. The built-in `openai` and `anthropic`
types register direct endpoints for standard-mode profiles and use the
`OPENAI_API_KEY` and `ANTHROPIC_API_KEY` variables. The credential flag
references an environment-variable name only; AgentBox never reads or stores
the key value. The GUI process and the OpenShell gateway must both run in an
environment where the referenced variables exist.

**Example**

```sh
agentbox provider setup --type gateway-bearer --host models.example.com \
  --credential ENMAAS_KEY --gateway office enmaas
```

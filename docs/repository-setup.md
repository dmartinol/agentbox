---
layout: docs
title: Repository setup
permalink: /repository-setup/
nav_sections:
  - title: Declare the project environment
    id: declare-the-project-environment
  - title: Validate and apply
    id: validate-and-apply
  - title: Extend the environment
    id: extend-the-environment
---

The sandbox needs the tools your project uses to build and test. A repository
manifest records those requirements so later runs can use the same declared setup.

## Declare the project environment

Commit `.agentbox/config.yaml` with the repository's extra packages, pinned
toolchains, image-build verification commands, and exact runtime network hosts.
Derive requirements from the repository's CI, build scripts, and dependency files.

With the bundled host skills installed, ask your assistant:

> Use agentbox-cli to set up AgentBox for this repository. Derive its tools, verification commands, and required hosts from the repository and CI.

The assistant authors the manifest from repository evidence. AgentBox does
not automatically infer it. Inspect the proposed changes before committing them.

## Validate and apply

Run from the project checkout:

```sh
agentbox repo setup --dry
agentbox repo setup
```

The first command validates the manifest and previews setup. The second builds
the repository image and generates the policy fragment. Existing profiles
provide the base agent images; they do not need to be rewritten. Project runs
automatically select the applicable repository image.

The GUI exposes the same flow under **Setup → Images → Repository images**.

The manifest's verification commands check the repository image during setup.
They are distinct from the acceptance checks a development task must run later.

## Extend the environment

Update the manifest and rerun setup when a task needs an additional tool or
network host. For a blocked network operation, use the run's surfaced denial
to identify the host, port, and executable; retain the rule in the repository
manifest if future runs need it.

Repositories without a manifest can use their profile image. AgentBox workers
in the orchestrated workflow require a prepared repository environment.

See [the manifest schema]({{ '/architecture/' | relative_url }}#repository-manifest)
for exact fields and validation rules, and
[the ADLC guide]({{ '/adlc/' | relative_url }}) for how the environment supports development work.

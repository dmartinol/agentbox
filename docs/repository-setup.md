---
layout: docs
title: Project configuration
permalink: /repository-setup/
nav_sections:
  - title: Describe the project environment
    id: describe-the-project-environment
  - title: Validate and apply
    id: validate-and-apply
  - title: Extend the environment
    id: extend-the-environment
---

Project configuration gives sandboxed agents the tools and network access the
repository needs. A repository manifest records those requirements so later
runs can use the same declared setup.

## Describe the project environment

Commit `.agentbox/config.yaml` with the repository's extra packages, pinned
toolchains, image-build verification commands, and exact runtime network hosts.
Derive requirements from the repository's CI, build scripts, and dependency files.

With the bundled host skills installed, ask your assistant:

> Use agentbox-cli to set up AgentBox for this repository. Derive its tools, verification commands, and required hosts from the repository and CI.

The assistant derives and authors the manifest from repository and CI
evidence; AgentBox does not automatically infer the configuration. Inspect the
proposed changes before committing them.

## Validate and apply

Run from the project checkout:

```sh
agentbox repo setup --dry
agentbox repo setup
```

The first command validates the manifest and previews setup. The second builds
a repository image for each selected harness on top of its base agent image,
then generates the network policy fragment from the declared hosts. Project
runs automatically select the applicable repository image.

The GUI exposes the same flow under **Setup → Images → Repository images**.

The manifest's verification commands check the repository image during setup.
They are distinct from the acceptance checks a development task must run later.

## Extend the environment

Update the manifest and rerun setup when a task needs an additional tool or
network host. For a blocked network operation, AgentBox surfaces the denied
host, port, and executable. Add the host to the active run's policy, then
optionally persist the same rule in project configuration for future runs.

Repositories without a manifest can use their profile image. AgentBox workers
in the orchestrated workflow require a prepared repository environment.

See [the manifest schema]({{ '/architecture/' | relative_url }}#repository-manifest)
for exact fields and validation rules, and
[the ADLC guide]({{ '/adlc/' | relative_url }}) for how the environment supports development work.

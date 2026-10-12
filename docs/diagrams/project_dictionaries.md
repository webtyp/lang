# Which dictionaries a project reads

A "project" is a module with a `config/` directory; its own dictionary is `config/lang.json`.
A library module that ships its own demo is **both**: it has `config/` (the demo) and a root
`lang.json` (the library dictionary its consumers read). Every diamond has an id;
[`docs/PLAN.md`](../PLAN.md) §3 maps each branch to its test.

```mermaid
flowchart TD
    A[load rootDir] --> P1{P1: has config/?}
    P1 -- no --> LIB[library: read rootDir/lang.json only]
    P1 -- yes --> PR[project: read config/lang.json]
    PR --> P2{P2: does rootDir/lang.json exist<br>and validate?}
    P2 -- yes --> OWN[add it as the FIRST library dictionary<br>module = main module path]
    P2 -- no --> DEPS
    OWN --> DEPS[add each dependency's lang.json<br>Discover order; broken ones skipped]
    DEPS --> USE[SyncTranslations, MissingTranslations,<br>BundleTranslations use the same list]
```

Precedence for a key and language (unchanged rule, now with the main module's library file):
`config/lang.json` value if non-empty → main module `lang.json` → dependencies in Discover order.

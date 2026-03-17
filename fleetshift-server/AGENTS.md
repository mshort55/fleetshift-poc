For how to...
- decide what logic to put where, see docs/internal-architecture.md
- implement instrumentation (observability: tracing, logging, metrics, ...), see docs/observer-pattern.md
- decide what package to use, see docs/package-structure.md
- write tests, see docs/testing.md
- write durable workflows and integrate with durable computing libraries, see docs/durable-workflows.md
- run tests, don't be afraid to run the whole suite even if some tests require containers.
- write constructors, use the "Option" function pattern, with exported option functions against a package private config struct.
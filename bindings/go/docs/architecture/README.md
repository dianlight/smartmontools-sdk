# Architecture Documentation

This directory contains Architecture Decision Records (ADRs) and other architectural documentation for the smartmontools-go project.

## What is an ADR?

An Architecture Decision Record (ADR) is a document that captures an important architectural decision made along with its context and consequences. ADRs help teams:

- Document the reasoning behind architectural choices
- Provide context for future developers
- Track the evolution of the system architecture
- Support informed decision-making

## ADR Index

### [ADR-001: SMART Data Access Approaches](./ADR-001-smart-access-approaches.md)

**Status:** Accepted (Option 1 default, Option 3 implemented as LibBackend)

**Summary:** Comprehensive analysis of four different approaches for accessing SMART data from storage devices:

1. **smartctl Command Wrapper (Default)** - Execute external smartctl binary and parse JSON output ✅ Implemented
2. **Direct ioctl Access** - Low-level kernel system calls for maximum performance ⏭️ Deferred
3. **Shared Library with FFI** - Use smartmontools as a shared library via purego ✅ Implemented as LibBackend
4. **Hybrid Approach** - Combine ioctl and shared library for optimal flexibility ⏭️ Future

The document includes:
- Detailed architecture diagrams for each approach
- Code examples and implementation details
- Performance comparisons and benchmarks
- Platform support matrix
- Security and maintenance considerations
- Recommendations for different use cases

---

### [ADR-002: Multi-Backend Architecture](./ADR-002-multi-backend-architecture.md)

**Status:** Accepted

**Summary:** Defines the `Backend` interface that abstracts SMART operations away from
any specific implementation. The existing exec/smartctl approach is `ExecBackend`.
The purego FFI approach is `LibBackend`. Both implement the same interface.
`Client` becomes a thin orchestrator. Includes a soft migration strategy across releases
v0.3–v1.0 with full backward compatibility at every step.

---

### [ADR-003: Shadow Mode and Telemetry](./ADR-003-shadow-mode-telemetry.md)

**Status:** Proposed

**Summary:** Introduces `ShadowBackend`, which runs a primary and a secondary backend
in parallel. The primary result is always returned to the caller; the secondary result
is compared asynchronously and differences are reported via a pluggable
`TelemetryReporter` interface. Supports four shadow modes (Disabled, Report, Fallback,
Validate) and includes bundled reporters for slog/tlog, OpenTelemetry, and HTTP webhooks.

---

### [ADR-004: Automated drivedb.h Tracking](./ADR-004-drivedb-autoupdate.md)

**Status:** Accepted

**Summary:** GitHub Actions workflow that monitors the upstream smartmontools
`lib/drivedb.h` file daily, detects changes via SHA-256 comparison, and automatically
opens a pull request. Embeds the upstream commit SHA and date as Go constants
(`DrivedbUpstreamCommit`, `DrivedbUpstreamDate`) for runtime inspection.

---

### [ROADMAP](./ROADMAP.md)

**Summary:** Phased release plan from the current exec-only v0.2.x to a fully
self-contained native Go backend at v1.0. Each phase introduces new backends
(ExecBackend refactor → ShadowBackend → IoctlBackend → LibBackend → NativeBackend)
while maintaining full backward compatibility throughout.

## ADR Template

When creating new ADRs, use this template:

```markdown
# ADR-XXX: [Title]

## Status

[Proposed | Accepted | Deprecated | Superseded]

## Context

What is the issue that we're seeing that is motivating this decision or change?

## Decision

What is the change that we're proposing and/or doing?

## Consequences

What becomes easier or more difficult to do because of this change?

## References

Links to related documents, discussions, or implementations.
```

## Contributing

When making significant architectural decisions:

1. Create a new ADR using the template above
2. Number it sequentially (ADR-002, ADR-003, etc.)
3. Discuss with the team before marking as "Accepted"
4. Update this index with a summary
5. Reference the ADR in related code changes

## Architecture Principles

The smartmontools-go project follows these architectural principles:

1. **Simplicity First**: Prefer simple, maintainable solutions over complex optimizations
2. **Cross-Platform**: Support Linux, macOS, and Windows where feasible
3. **Minimal Dependencies**: Avoid unnecessary external dependencies
4. **Performance Awareness**: Design with performance in mind, but don't sacrifice maintainability
5. **Clear Abstractions**: Provide clean, well-documented interfaces
6. **Backward Compatibility**: Maintain API stability within major versions
7. **Security**: Consider security implications of all architectural decisions

## Related Documentation

- [API Documentation](../../APIDOC.md) - Complete API reference
- [README](../../README.md) - Project overview and quick start
- [Examples](../../examples/) - Usage examples

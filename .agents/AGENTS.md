# Error Handling and Logging Policy

This workspace adheres to a strict Error Handling and Logging Policy to eliminate log noise and redundant error reporting in request-driven code paths while maintaining strong observability for critical runtime infrastructure.

## 1. Data Access Layer (Repositories, DB Helpers, Storage Utilities)

Follow the **"Wrap at Bottom, Log at Top"** principle.

* **Never log errors** in repositories, storage layers, or database helpers.
* Add context and propagate errors using:
  ```go
  fmt.Errorf("operation description: %w", err)
  ```
* Lower layers are responsible for enriching errors, not emitting logs.
* Avoid duplicate logging of the same failure across multiple layers.

## 2. Request Boundaries (HTTP Handlers, Admin Handlers, gRPC Handlers, CLI Entry Points)

These are request execution boundaries.

* **Log errors exactly once** at the boundary.
* Retrieve the full wrapped error chain when logging (`.Err(err)`).
* Convert errors into the appropriate response format (HTTP, gRPC, CLI, etc.).
* Do not re-log errors already logged at the same boundary.
* Boundary layers are responsible for observability of request-driven workflows.

## 3. Runtime / Execution Engines (Load Balancer, Reverse Proxy, Route Loader, Health Checker, Background Workers)

These are long-running operational components responsible for system runtime behavior.

* **Log failures immediately** at the point of detection.
* Operational events affecting gateway health, routing, traffic flow, configuration loading, or background processing must be visible in logs.
* Errors may be both logged and returned/wrapped when appropriate.
* Prioritize operational observability over strict log deduplication in these infrastructure components.

## 4. Service Layer (Business Logic)

* **Generally do not log errors.**
* Wrap and propagate errors with additional business context.
* Allow the boundary layer to decide how and when to log.
* Exception: log only when required for critical operational visibility that cannot be provided elsewhere.

## 5. Models, Constants, DTOs, Configuration Structures

* **No logging.**
* No error handling responsibilities.
* These are passive data structures.

## 6. Pure Logic and Utility Packages

* Prefer returning errors rather than logging them.
* Log only when detecting internal state inconsistencies, invariant violations, or conditions that could affect system reliability and are otherwise difficult to observe.

## 7. General Rules

* Wrap errors at lower layers.
* Log errors once at execution boundaries.
* Avoid duplicate logs for request-driven flows.
* Use `%w` when propagating errors to preserve the error chain.
* Runtime infrastructure components may log locally to maintain operational visibility.
* Every log entry should provide actionable operational context.
* Optimize for high signal-to-noise ratio while preserving observability of critical gateway operations.

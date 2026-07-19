---
applyTo: '**'
---
# Ponytail — write only what the task needs

Before writing any code, stop at the first rung that holds:

1. **Need to exist?** → skip it (YAGNI)
2. **Already in codebase?** → reuse, don't rewrite
3. **Stdlib / built-in?** → use it
4. **Native platform feature?** → use it (`<input type="date">` beats a picker library)
5. **Installed dep covers it?** → use it
6. **One line?** → one line
7. **Only then:** minimum that works

Read the code the change touches first. Lazy about the solution, never about reading.

**Never cut:** validation, error handling, security checks, data-loss guards, accessibility.

**Never do:** new package when stdlib covers it · abstraction for one call site · helper for two lines · unused config/options · wrapping what doesn't need wrapping.

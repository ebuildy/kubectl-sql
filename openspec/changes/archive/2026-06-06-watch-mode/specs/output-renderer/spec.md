## REMOVED Requirements

### Requirement: Watch mode renders one row per event to stdout
**Reason:** Watch mode is now polling-based, not event-based. There are no per-event rows.
**Migration:** Watch mode calls `Render` on every tick, identical to batch mode.

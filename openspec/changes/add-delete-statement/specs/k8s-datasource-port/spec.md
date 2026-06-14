## MODIFIED Requirements

### Requirement: Port exposes listing, schema, resource discovery, and deletion
The `DataSource` port SHALL provide: paginated listing of a resource's objects as
`[]map[string]any`; schema inference for a resource as `[]schema.Field`; enumeration of all
queryable resources (names plus aliases) for `SHOW TABLES` and completion; and deletion of a
single object identified by its resource, namespace, and name, with a domain `DeleteOptions`
value (grace period, force, propagation policy) expressed in plain Go. The delete operation is
the only mutating method on the port; all other methods remain read-only. The concrete client-go
binding for delete SHALL be confined to the adapter package like every other library call, and
SHALL translate `DeleteOptions` into the library's delete options.

#### Scenario: Paginated list returns plain objects
- **WHEN** a consumer lists a resource through the port with a page size
- **THEN** it receives the objects as `[]map[string]any` without any client-go types crossing the boundary

#### Scenario: Schema inference returns domain fields
- **WHEN** a consumer infers a resource's schema through the port
- **THEN** it receives `[]schema.Field` (the existing field model), with server-managed metadata fields omitted as before

#### Scenario: Resource enumeration backs SHOW TABLES
- **WHEN** `SHOW TABLES` is executed
- **THEN** the table list is produced from the port's resource enumeration, identical to the current output

#### Scenario: Delete removes a single object by identity
- **WHEN** a consumer calls the port's delete with a resolved `Resource`, a namespace, an object name, and a `DeleteOptions`
- **THEN** the adapter issues a dynamic-client delete for that object honouring the options and returns nil on success or a wrapped error on failure, with no client-go types crossing the boundary

#### Scenario: Delete options are translated by the adapter
- **WHEN** a consumer passes `DeleteOptions` with a grace period and/or propagation policy
- **THEN** the adapter maps them onto the client-go delete options (e.g. `GracePeriodSeconds`, `PropagationPolicy`) without those library types appearing in the port signature

#### Scenario: Delete signature is library-free
- **WHEN** the `internal/port/datasources/k8s` package is compiled
- **THEN** the delete method (including `DeleteOptions`) uses only standard-library and domain types and imports no `k8s.io/*` package

# Database as a Service — Product Manifesto

## We believe databases are products, not infrastructure

Databases are not servers.
They are not instances.
They are not clusters.

A database is a **long-lived business asset** that holds data critical to a product, a team, and an organization. Treating databases as infrastructure primitives forces product teams to make decisions they should not make, and prevents platform teams from evolving systems safely.

This platform exists to correct that mismatch.

---

## The database is the primary abstraction

This platform exposes **databases**, not PostgreSQL servers.

Consumers of the platform interact with:

* databases
* ownership
* lifecycle
* guarantees

They do **not** interact with:

* instances
* clusters
* replication topology
* storage layout

Infrastructure is an implementation detail.
The database is the contract.

---

## Database as a Service means freedom through abstraction

Providing Databases as a Service means that:

* product teams are free to focus on data and application logic
* platform teams are free to evolve infrastructure without breaking consumers
* the organization gains leverage over time instead of accumulating rigidity

The platform may change *how* a database is hosted.
It must not change *what* the database represents.

---

## Responsibility is explicit, not implicit

This platform **explicitly defines a responsibility model**.

It is not neutral.
It is not negotiable.
It is part of the product.

### Product teams are responsible for:

* schema design
* data correctness
* application-level migrations
* using the database according to documented guarantees

### Platform teams are responsible for:

* availability and durability
* infrastructure changes
* safe execution of data-moving operations
* evolving the system without downtime whenever possible

This division is intentional.
Without it, Database as a Service collapses into confusion.

---

## Ownership is required, not optional

Every database must have:

* a clear owner
* a clear purpose
* a defined lifecycle

Ownership is not bureaucracy.
It is the minimum condition for safety, accountability, and evolution.

A database without an owner is technical debt by definition.

---

## Database lifecycle is a first-class concern

Databases are created, evolve, move, and eventually disappear.

This platform treats lifecycle management as a **core capability**, not an afterthought:

* creation
* movement between underlying systems
* deprecation
* archival
* deletion

Moving a database between PostgreSQL systems **without downtime** is a platform responsibility.
Changing schemas is not.

The platform manages **data placement**, not **data shape**.

---

## Opinionated by design

This platform is intentionally opinionated.

It enforces:

* a clear responsibility model
* explicit ownership
* well-defined lifecycles
* constraints that protect the system as a whole

It does not try to support every workflow, every organization, or every philosophy.

If these opinions do not match an organization’s values, this platform is not for them.

---

## Open source does not mean neutral

This platform is designed to be open source **without compromising its vision**.

Being open source does not require being generic.
It does not require infinite configurability.
It does not require hiding opinions.

This project is open source so organizations can:

* adopt the model
* learn from it
* build upon it

Not so they can dilute it.

---

## Infrastructure is a means, not the goal

The platform may rely on:

* managed services
* operators
* internal systems
* future abstractions

These choices are internal and replaceable.

What matters is the **product contract**, not the implementation.

---

## Our goal

Our goal is to enable organizations to build **their own internal Database as a Service**:

* aligned with their structure
* explicit in responsibilities
* safe to operate at scale
* designed for long-term evolution

Not PostgreSQL as a Service.
**Databases as a Service.**

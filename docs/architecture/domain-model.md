# Domain model

The installation row records the identity of the deployed database. A future singleton organization profile owns installation-level business settings. Internal divisions contain branches, while departments and employees define reporting and access boundaries. Locations model physical containment through building, floor, room and storage levels.

Asset classification owns reusable categories, manufacturers and models. Asset identity and ownership remain separate from employee custody. The assignment history records past custody and a partial unique index will enforce exclusive active custody. Financial values use exact decimals. Audit events preserve actor, operation, affected record and timestamp.

Milestone 1 implements only installation identity and migration history.

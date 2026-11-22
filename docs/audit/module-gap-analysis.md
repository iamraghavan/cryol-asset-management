# Module completion matrix

| Module | Baseline | Required API and database work | UI work | Priority |
| --- | --- | --- | --- | --- |
| Asset registry | Partial. List and create | Add record read, update, archive and restore, typed inputs, audit history and optimistic revision checks. Keep legacy fields and additive migrations. | Search, filter, paginated table and record drawers | First |
| Organization profile | Partial. Singleton read/write | Existing API remains compatible. | Integrate into console shell | First |
| Authentication | Partial. Local sessions and sign-in | Add account lifecycle and organizational permissions in a follow-up. | Preserve login and add account menu | First |
| Dashboard | Partial. One real aggregate | Add archive-state counts alongside total. Add custody figures once custody exists. | Real metrics linked to filtered inventory | First |
| Category and organization directory | Missing | Relational lookup, hierarchy and dependent archive checks | Reuse reference-module components | Next |
| Assignment and lifecycle | Missing | Transactional custody, eligibility rules, foreign keys and conflict constraints | Assign, return and transfer interactions | Next |
| Reports and exports | Missing | Scoped aggregation and export contracts | Report list, filters and progress | Later |

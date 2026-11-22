# System overview

One Cryol installation serves one organization. Browsers connect to a Go service over the LAN. The service owns the HTTP API and serves the built React application from local files. PostgreSQL stores authoritative records. Protected storage will hold attachments outside the frontend directory. PostgreSQL and the application may run on separate hosts on the customer's network.

The foundation uses chi, a pgx connection pool, Goose migrations embedded in both binaries, and sqlc for typed installation queries. The server checks its schema before listening. A separate CLI applies migrations with provisioned database credentials. Configuration comes from the service environment. The default listener binds to loopback. Production service wrappers and client-managed HTTPS are planned deployment work.

Future workers will claim durable jobs through PostgreSQL and will use the same scoped service layer as HTTP handlers. Asset custody changes will use a transaction covering the current assignment, asset status and history. Backups will coordinate the database and protected storage so a restore can reconcile attachment metadata and files.

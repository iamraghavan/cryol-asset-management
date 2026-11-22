-- name: InstallationExists :one
SELECT EXISTS (
    SELECT 1 FROM installation_info WHERE singleton
);

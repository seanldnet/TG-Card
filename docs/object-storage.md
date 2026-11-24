# S3-compatible media storage

## Scope

The media storage backend is for public product/category images, administrator avatars, and the site logo. It does not store payment data, order payloads, bot tokens, or private support-ticket attachments. Support attachments continue to be fetched from Telegram through the authenticated ticket/message ownership check and returned with `private, no-store`.

The application keeps serving existing `/uploads/...` URLs. When S3 storage is active, new media is written to the configured bucket and reads fall back to the local `DATA_DIR/uploads` directory when an object is not present in the bucket. This allows a gradual migration without rewriting database URLs.

## Configuration

Open **Website Settings → Object Storage**:

- **Local** keeps the existing `DATA_DIR/uploads` backend.
- **S3-compatible** requires an HTTPS (HTTP is accepted for private development endpoints) origin URL, an existing bucket, Access Key, and Secret Key. Endpoint paths, embedded credentials, query strings, and fragments are rejected.
- Region is optional for providers that do not require one. Path prefix is optional and is isolated from other objects in the bucket.
- Credentials are encrypted with the application's existing AES-GCM secret manager. The settings API returns only configured flags, never credentials.
- The bucket does not need public access. The application streams media through its existing `/uploads/...` route.

The configured identity needs bucket inspection/list permission for the connection check, plus object read, write, delete, and list permissions for normal operation and cleanup. Use a dedicated least-privilege key scoped to the chosen bucket and prefix. Keep the application's persistent `DATA_DIR` intact because it contains the key used to encrypt the credentials.

## Migration

1. Configure the S3-compatible backend and run **Test Connection**.
2. Save the configuration. New uploads and subsequent upstream image syncs now write to S3.
3. Run **Migrate Local Historical Files**. The application copies only regular files under the existing public `avatars/`, `site/`, and `upstream/` namespaces.
4. Every copied object is read back and SHA-256 checked before it is counted as migrated.
5. Local source files are intentionally retained. Do not delete the old `DATA_DIR/uploads` files until a separate backup and rollback plan has been verified.

A failed copy or verification aborts the request and leaves all local source files intact. Re-running the migration is safe: object keys are deterministic and content is verified after each write.

To switch back to local storage, select **Local** and run **Migrate Object Storage to Local and Switch**. The application copies all supported public media namespaces back to the local volume, reads each file back, verifies SHA-256, then changes the active backend. S3 source objects and credentials are retained. Directly changing an active S3 backend to Local, or changing its endpoint/bucket/prefix, is rejected until this reverse migration completes.

## Compatibility and security boundaries

- Existing product/category image URLs and site-logo/avatar URLs remain unchanged.
- Local fallback reads support a staged migration and rollback.
- Upstream images remain content-addressed by SHA-256.
- Local storage rejects traversal paths and symbolic links.
- The S3 bucket can remain private; the application proxy serves public media.
- Support-ticket attachments are not copied to S3 and are never exposed through the public `/uploads/` handler.
- No database schema migration is required; settings use the existing `settings` table.

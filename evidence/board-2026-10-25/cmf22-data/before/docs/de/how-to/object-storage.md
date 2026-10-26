# Uploads mit S3 speichern

Applications konsumieren S3-kompatible Bindungen, ohne das Storage-Produkt im portablen Intent festzuschreiben.

Mit `baha` und konfiguriertem Runtime-Target:

```bash
baha app new media-api --stack go --http --s3
cd media-api
baha plan
baha up -e dev
baha app env --format json
```

Das Go-Scaffold bereitet AWS SDK und `HeadBucket`-Verifikation vor; es erzeugt keinen Upload-Endpunkt.

| Bindung | Zweck |
| --- | --- |
| `S3_ENDPOINT`, `S3_BUCKET` | Endpunkt und Application-Bucket |
| `AWS_REGION` | SDK-Region |
| `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` | Geschützte Application-Zugangsdaten |
| `AWS_CA_BUNDLE` | TLS-Vertrauen |

Im eigenen Handler mit `s3Client`, `bucket`, Request-Kontext `ctx` und `io.Reader` namens `photo`:

```go
key := "products/mug-42/photo.jpg"
_, err := s3Client.PutObject(ctx, &s3.PutObjectInput{
    Bucket: &bucket,
    Key: &key,
    Body: photo,
})
```

SDK-Fehler behandeln, bevor der Objektschlüssel in Geschäftsdaten landet. Zugangsdaten bleiben in Bindungen; speichere Objektschlüssel statt credential-haltiger URLs. `baha doctor` prüft Storage, Uploads gehören in [Backup/Restore](backup-restore.md).

[Kanonische Anleitung (EN)](https://mcpdev80.github.io/baseharbor/how-to/object-storage/).

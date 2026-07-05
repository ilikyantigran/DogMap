-- Email confirmation on registration. New accounts start unverified; a user
-- becomes verified by following the emailed link (VerifyEmail). Login rejects
-- unverified accounts. Backfill default is false — existing MVP rows (if any)
-- will need to re-verify, which is acceptable pre-launch.

ALTER TABLE auth.credentials
    ADD COLUMN IF NOT EXISTS email_verified boolean NOT NULL DEFAULT false;

# Sign-in rules: `PASSKEY_REQUIRED`, `SSO_REQUIRED` and `passkeys_required`

The error catalogue adds `PASSKEY_REQUIRED` (403, api, dashboard and edge) for a session an
emailed code or a password began acting in a business that asks everyone to sign in with a
passkey or company sign-in, and `SSO_REQUIRED` (403, api, dashboard and edge) for a session the
business's own identity provider did not make acting in a business that requires it, whatever
the member's email domain. Each fix says to sign out and sign in the stronger way. Org settings
(`OrgSettings`, `OrgSettingsRequest`) gain `passkeys_required`, a boolean only an owner changes.

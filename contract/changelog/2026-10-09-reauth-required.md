# REAUTH_REQUIRED

The error catalogue adds `REAUTH_REQUIRED` (403, sign-in host) for a change to how a person signs
in (adding or removing a passkey, setting, changing or removing a password) asked for more than
ten minutes after they last proved who they are. Confirming at `/passkeys/confirm` opens the next
ten minutes.

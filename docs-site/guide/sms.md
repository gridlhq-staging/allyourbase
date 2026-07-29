# SMS
<!-- audited 2026-07-27 -->

AYB supports SMS one-time-password login, SMS MFA, authenticated application messaging, and admin delivery operations. This page owns provider setup and operational details; see [Authentication](/guide/authentication) for the broader authentication model.

## Local setup with the log provider

The `log` provider writes message bodies, including development OTPs, to the AYB server log instead of contacting a carrier. Do not use it for production delivery.

```toml
[auth]
enabled = true
sms_enabled = true
sms_provider = "log"
sms_code_length = 6
sms_code_expiry = 300
sms_max_attempts = 3
sms_daily_limit = 1000
sms_allowed_countries = ["US", "CA"]
```

The equivalent enable/provider environment overrides are:

```bash
AYB_AUTH_ENABLED=true \
AYB_AUTH_SMS_ENABLED=true \
AYB_AUTH_SMS_PROVIDER=log \
ayb start
```

The numeric limits, country allowlist, and test phone numbers are configured in `ayb.toml`; they do not have `AYB_AUTH_*` environment mappings.

## Configuration and limits

| Setting | Meaning |
| --- | --- |
| `auth.sms_enabled` | Enables SMS auth and wires the selected provider. Requires `auth.enabled = true`. |
| `auth.sms_provider` | One of `log`, `twilio`, `plivo`, `telnyx`, `msg91`, `sns`, `vonage`, or `webhook`. |
| `auth.sms_code_length` | OTP length, from 4 through 8 digits. |
| `auth.sms_code_expiry` | OTP lifetime, from 60 through 600 seconds. |
| `auth.sms_max_attempts` | Failed checks allowed for an OTP before it is deleted. |
| `auth.sms_daily_limit` | First-factor auth OTP request limit per database day; `0` means unlimited. Failed provider sends still consume the count. |
| `auth.sms_allowed_countries` | Allowed ISO 3166-1 alpha-2 country codes, such as `US` or `CA`. |
| `auth.sms_test_phone_numbers` | Map of phone numbers to predetermined codes for controlled testing. First-factor auth entries bypass provider delivery, normalization, country checks, and the daily limit. MFA uses the predetermined code after its normal phone validation. |

Phone inputs are normalized to E.164. Requests outside `sms_allowed_countries` are rejected by messaging sends. Auth OTP requests for blocked countries return the same generic success response as accepted requests to avoid account or phone-number enumeration.

## Production providers

The examples in this section are configuration examples only. They contact real providers when AYB sends a message and may incur charges.

| Provider | Required `[auth]` fields | Environment equivalents |
| --- | --- | --- |
| `twilio` | `twilio_sid`, `twilio_token`, `twilio_from` | `AYB_AUTH_TWILIO_SID`, `AYB_AUTH_TWILIO_TOKEN`, `AYB_AUTH_TWILIO_FROM` |
| `plivo` | `plivo_auth_id`, `plivo_auth_token`, `plivo_from` | `AYB_AUTH_PLIVO_AUTH_ID`, `AYB_AUTH_PLIVO_AUTH_TOKEN`, `AYB_AUTH_PLIVO_FROM` |
| `telnyx` | `telnyx_api_key`, `telnyx_from` | `AYB_AUTH_TELNYX_API_KEY`, `AYB_AUTH_TELNYX_FROM` |
| `msg91` | `msg91_auth_key`, `msg91_template_id` | `AYB_AUTH_MSG91_AUTH_KEY`, `AYB_AUTH_MSG91_TEMPLATE_ID` |
| `sns` | `aws_region`; credentials come from the AWS credential chain | `AYB_AUTH_AWS_REGION` plus the standard AWS credential environment |
| `vonage` | `vonage_api_key`, `vonage_api_secret`, `vonage_from` | `AYB_AUTH_VONAGE_API_KEY`, `AYB_AUTH_VONAGE_API_SECRET`, `AYB_AUTH_VONAGE_FROM` |
| `webhook` | `sms_webhook_url`, `sms_webhook_secret` | `AYB_AUTH_SMS_WEBHOOK_URL`, `AYB_AUTH_SMS_WEBHOOK_SECRET` |

For example, a Twilio configuration is:

```toml
[auth]
enabled = true
sms_enabled = true
sms_provider = "twilio"
twilio_sid = "AC..."
twilio_token = "..."
twilio_from = "+14155550100"
sms_allowed_countries = ["US", "CA"]
```

The webhook provider sends `{"to":"...","body":"..."}` as JSON and signs the request body in `X-Webhook-Signature` with HMAC-SHA256. Its endpoint must return a successful HTTP status and may return `{"message_id":"..."}`.

## SMS OTP authentication

Request a code:

```bash
curl -sS -X POST http://127.0.0.1:8090/api/auth/sms \
  -H "Content-Type: application/json" \
  -d '{"phone":"+14155552671"}'
```

`POST /api/auth/sms` returns `200` with a generic message when the phone format is valid, even when the country is blocked, delivery fails, or the daily limit is reached. Inspect server logs and [SMS health](#admin-health-messages-and-test-send) for operations; do not infer delivery from this response.

Confirm the code:

```bash
curl -sS -X POST http://127.0.0.1:8090/api/auth/sms/confirm \
  -H "Content-Type: application/json" \
  -d '{"phone":"+14155552671","code":"123456"}'
```

`POST /api/auth/sms/confirm` consumes a valid code. It returns access and refresh tokens for a normal login, or an MFA-pending token when the user has an enrolled factor.

## SMS MFA

Enrollment endpoints require a normal user bearer token. Anonymous users cannot enroll MFA. If the account already has an enabled factor, enrolling another factor requires an AAL2 session.

```bash
# Begin enrollment
curl -sS -X POST http://127.0.0.1:8090/api/auth/mfa/sms/enroll \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"phone":"+14155552671"}'

# Confirm enrollment
curl -sS -X POST http://127.0.0.1:8090/api/auth/mfa/sms/enroll/confirm \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"phone":"+14155552671","code":"123456"}'
```

After a first-factor login returns `mfa_pending: true`, use its `mfa_token` for challenge and verification:

```bash
curl -sS -X POST http://127.0.0.1:8090/api/auth/mfa/sms/challenge \
  -H "Authorization: Bearer $MFA_TOKEN"

curl -sS -X POST http://127.0.0.1:8090/api/auth/mfa/sms/verify \
  -H "Authorization: Bearer $MFA_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"code":"123456"}'
```

The four routes are:

- `POST /api/auth/mfa/sms/enroll`
- `POST /api/auth/mfa/sms/enroll/confirm`
- `POST /api/auth/mfa/sms/challenge`
- `POST /api/auth/mfa/sms/verify`

Codes expire according to `sms_code_expiry`, are single-use, and are deleted after `sms_max_attempts` failed checks. SMS MFA verification also participates in the cumulative MFA lockout described in [Authentication](/guide/authentication#operational-limits).

::: warning Security classification
SMS MFA is a NIST Restricted authenticator because SIM-swap and signaling-network attacks can defeat it. AYB issues AAL2 tokens after successful SMS MFA for a consistent authorization model, but TOTP is the preferred factor for higher-risk or compliance-sensitive use cases.
:::

## Application messaging

These endpoints require user authentication; API keys also need write scope to send:

- `POST /api/messaging/sms/send`
- `GET /api/messaging/sms/messages`
- `GET /api/messaging/sms/messages/{id}`

Send and persist a message:

```bash
curl -sS -X POST http://127.0.0.1:8090/api/messaging/sms/send \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"to":"+14155552671","body":"Your order is ready."}'
```

The send route validates the E.164 phone number, country allowlist, non-empty body, and a 1,600-byte maximum. It writes a pending record to `_ayb_sms_messages`, calls the provider, and then records the provider message ID and resulting status or marks the record failed. List/get routes return only the authenticated user's records.

### Twilio delivery status

`POST /api/webhooks/sms/status` accepts Twilio's form-encoded delivery callback and advances the stored message status without allowing lifecycle regressions. Configure Twilio to call this endpoint. AYB verifies `X-Twilio-Signature` with `auth.twilio_token` and fails closed when verification is unavailable.

## Admin health, messages, and test send

Admin routes require an admin bearer token:

- `GET /api/admin/sms/health` — auth OTP sent, confirmed, failed, and conversion-rate aggregates for today, 7 days, and 30 days.
- `GET /api/admin/sms/messages` — all persisted application messages, paginated with `page` and `perPage` (`50` by default, `200` maximum).
- `POST /api/admin/sms/send` — validates and sends a provider test message.

The admin send route does not persist a message and does not affect the messaging history. With the local log provider:

```bash
curl -sS -X POST http://127.0.0.1:8090/api/admin/sms/send \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"to":"+14155552671","body":"Local SMS provider check"}'
```

For the `log` provider, a successful response reports status `logged`; the full body appears in the AYB server log.

# Admin elevation process

Admin elevation temporarily gives a standard macOS user administrator privileges under the organization's Mosyle policy. The process consists of obtaining the request context, validating the policy, submitting a justification, and waiting for Mosyle to apply access on the Mac.

The API details below describe the web endpoints used by this repository's observed integration. These are undocumented endpoints, not a verified public API contract. Examples use placeholder values.

## Authentication and HTTP requests

Requests use an existing authenticated Mosyle session through cookies and are sent to `https://mybusiness.mosyle.com`. No API key or bearer token is used in this flow. An `IntegrityToken` obtained from the request form is included in the subsequent policy and elevation requests; it does not replace session authentication.

All three calls use HTTPS POST with URL-encoded form bodies and these headers:

```http
Content-Type: application/x-www-form-urlencoded
Origin: https://mybusiness.mosyle.com
Referer: https://mybusiness.mosyle.com/
X-Requested-With: XMLHttpRequest
Cookie: <applicable session cookies>
```

The field lists below show decoded values for readability. On the wire, each body is encoded as `key=value&key=value`, with special characters escaped.

## 1. Obtain the elevation request context

```http
POST /screens/scules/selfservice/index.php
```

| Form field | Value |
| --- | --- |
| `deviceudid` | The Mac's `IOPlatformUUID` |
| `oswebview` | `mac` |
| `build` | Installed Self-Service app's `CFBundleVersion` |
| `HasUpperBar` | `0` |
| `tab_opened` | `ON_DEMAND` |

The response is HTML. The elevation action is identified by an element whose `onclick` contains `requestOndemandAdmin(this)`. Its attributes supply the context for the next two calls:

| HTML attribute | Subsequent form field |
| --- | --- |
| `data-deviceudid` | Checked against the local device identifier; that identifier is sent as `deviceudid` |
| `data-profileID` | `ProfileID` |
| `data-idcompany` | `idcompany` and `usertab_current_idcompany` |
| `data-IntegrityToken` | `IntegrityToken` |

HTML attribute names are treated case-insensitively. Exactly one matching action must exist, all four attributes must be populated, and the returned device identifier must match the local Mac, ignoring case. Otherwise, the flow stops before submission.

This call obtains context only; it does not request elevation.

## 2. Validate the elevation policy

```http
POST /Controller/selfservice.php
```

| Form field | Value |
| --- | --- |
| `operation` | `validate_permission_profile` |
| `mapping` | `OnDemandController` |
| `deviceudid` | Local device identifier |
| `ProfileID` | Profile ID from the form |
| `idcompany` | Company ID from the form |
| `IntegrityToken` | Token from the form |
| `usertab_current_os` | `mac` |
| `usertab_current_idcompany` | Same company ID |

The relevant response fields have this shape:

```json
{
  "ProfileOptions": {
    "RequestSettings": {
      "Type": "AllowUsers",
      "RequestMinutes": 10
    }
  }
}
```

`Type` must be `AllowUsers` for the flow described here. Other policy types, including workflows requiring separate administrator approval, are not handled by this integration. A missing type is treated as an unsupported response.

`RequestMinutes` defines the policy's elevation duration. It must be a positive integer and may arrive as a JSON number or numeric string. The example's 10 minutes is illustrative, not a fixed duration.

Policy validation does not itself submit an elevation request.

## 3. Submit the elevation request

```http
POST /Controller/selfservice.php
```

The request reuses the device, profile, company, and integrity-token context:

| Form field | Value |
| --- | --- |
| `operation` | `validate_ondemand_request` |
| `deviceudid` | Local device identifier |
| `UDID` | Same device identifier |
| `ProfileID` | Profile ID from the form |
| `idcompany` | Company ID from the form |
| `IntegrityToken` | Token from the form |
| `usertab_current_os` | `mac` |
| `usertab_current_idcompany` | Same company ID |
| `justification` | The user's nonempty reason for requesting access |

`mapping` is omitted from this call. No requested duration is submitted: the duration comes from the validated policy.

An example acceptance response is:

```json
{
  "approved": 1,
  "PushEnable": false
}
```

The response is interpreted as follows:

| `approved` value | Interpretation |
| --- | --- |
| `1` or `"1"` | Request accepted |
| `0`, `"0"`, or `false` | Request denied |
| Missing or any other value | Submission outcome unknown |

`PushEnable` is optional and is parsed, but it is not used as evidence that elevation has activated. Its presence or value does not establish how Mosyle delivers the change to the Mac.

Acceptance confirms that Mosyle accepted the request for processing. It does not yet confirm administrator privileges on the device.

## 4. Activate administrator privileges

Mosyle applies elevation to the Mac. Access is confirmed when the user becomes a member of the local `admin` group. A local membership check is:

```sh
/usr/sbin/dseditgroup -o checkmember -m <username> admin
```

There may be a delay between API acceptance and the membership change. This integration does not use an API endpoint to confirm activation; it observes local membership. If the user is already an administrator, an additional elevation request is unnecessary.

Admin membership lets the user authorize operations requiring administrator privileges. It does not automatically run every application or command as root, and macOS may still ask for authentication.

The observed flow does not establish Mosyle's internal mechanism for applying the change, such as which device-side component performs it or which delivery channel triggers it.

## 5. Expire the temporary access

Mosyle manages automatic revocation according to the configured elevation duration. Once temporary access is revoked, the user returns to standard-user permissions, assuming they have no other source of administrator access.

There is no renewal or revocation API call in this flow. Local membership cannot reveal the exact expiry time or when Mosyle starts its duration timer. Automatic expiry still needs independent validation for the employee-account flow documented by this project.

## Failures and uncertain outcomes

The integration expects HTTP 200 responses. HTTP 401 and redirects are treated as session failures; redirects are not followed. HTTP 403 is treated as denial.

Before submission, transport failures or unexpected responses stop preparation. During submission, a transport failure, an unexpected HTTP status other than the session/denial cases above, an unreadable response, or malformed JSON can leave the outcome unknown: Mosyle may have processed the request even though acceptance was not received.

Requests have a 20-second HTTP timeout. The elevation submission is not automatically retried and sends no idempotency header. After an uncertain result, check local membership and allow time for activation before deciding whether to submit again.

Stopping a wait or closing the requesting process does not cancel an accepted request. Elevation may still activate afterward.

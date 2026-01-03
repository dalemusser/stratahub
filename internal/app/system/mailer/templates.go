// internal/app/system/mailer/templates.go
package mailer

import (
	"bytes"
	"fmt"
	"html/template"
)

// VerificationEmailData holds data for verification email templates.
type VerificationEmailData struct {
	SiteName  string
	Code      string
	MagicLink string
	ExpiresIn string // e.g., "10 minutes"
}

// BuildVerificationEmail creates a verification email with both HTML and text bodies.
func BuildVerificationEmail(data VerificationEmailData) Email {
	return Email{
		To:       "", // Set by caller
		Subject:  fmt.Sprintf("Your %s verification code", data.SiteName),
		TextBody: buildVerificationText(data),
		HTMLBody: buildVerificationHTML(data),
	}
}

func buildVerificationText(data VerificationEmailData) string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("Your %s verification code is: %s\n\n", data.SiteName, data.Code))
	buf.WriteString("Or click this link to sign in:\n")
	buf.WriteString(data.MagicLink + "\n\n")
	buf.WriteString(fmt.Sprintf("This code expires in %s.\n\n", data.ExpiresIn))
	buf.WriteString("If you did not request this code, you can safely ignore this email.\n")
	return buf.String()
}

func buildVerificationHTML(data VerificationEmailData) string {
	tmpl := template.Must(template.New("verification").Parse(verificationHTMLTemplate))
	var buf bytes.Buffer
	_ = tmpl.Execute(&buf, data)
	return buf.String()
}

const verificationHTMLTemplate = `<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Verification Code</title>
</head>
<body style="margin: 0; padding: 0; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; background-color: #f3f4f6;">
  <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="background-color: #f3f4f6;">
    <tr>
      <td align="center" style="padding: 40px 20px;">
        <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="max-width: 480px; background-color: #ffffff; border-radius: 8px; box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);">
          <!-- Header -->
          <tr>
            <td style="padding: 32px 32px 24px; text-align: center; border-bottom: 1px solid #e5e7eb;">
              <h1 style="margin: 0; font-size: 24px; font-weight: 600; color: #4f46e5;">{{.SiteName}}</h1>
            </td>
          </tr>

          <!-- Content -->
          <tr>
            <td style="padding: 32px;">
              <p style="margin: 0 0 24px; font-size: 16px; color: #374151; line-height: 1.5;">
                Your verification code is:
              </p>

              <!-- Code Box -->
              <div style="background-color: #f3f4f6; border-radius: 8px; padding: 24px; text-align: center; margin-bottom: 24px;">
                <span style="font-size: 32px; font-weight: 700; letter-spacing: 8px; color: #1f2937; font-family: 'Courier New', monospace;">{{.Code}}</span>
              </div>

              <p style="margin: 0 0 24px; font-size: 14px; color: #6b7280; text-align: center;">
                Or click the button below to sign in:
              </p>

              <!-- Button -->
              <table role="presentation" width="100%" cellspacing="0" cellpadding="0">
                <tr>
                  <td align="center">
                    <a href="{{.MagicLink}}" style="display: inline-block; padding: 14px 32px; background-color: #4f46e5; color: #ffffff; text-decoration: none; font-size: 16px; font-weight: 500; border-radius: 6px;">
                      Sign In
                    </a>
                  </td>
                </tr>
              </table>

              <p style="margin: 24px 0 0; font-size: 13px; color: #9ca3af; text-align: center;">
                This code expires in {{.ExpiresIn}}.
              </p>
            </td>
          </tr>

          <!-- Footer -->
          <tr>
            <td style="padding: 24px 32px; background-color: #f9fafb; border-top: 1px solid #e5e7eb; border-radius: 0 0 8px 8px;">
              <p style="margin: 0; font-size: 12px; color: #9ca3af; text-align: center;">
                If you did not request this code, you can safely ignore this email.
              </p>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`

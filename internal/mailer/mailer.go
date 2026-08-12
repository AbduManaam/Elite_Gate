package mailer

import "context"

type Mailer interface {
	SendPasswordReset(
		ctx context.Context,
		recipient string,
		resetURL string,
	) error

	SendEmailVerification(
		ctx context.Context,
		recipient string,
		verificationURL string,
	) error
}

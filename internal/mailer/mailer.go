package mailer

import "context"

type Mailer interface {
	SendPasswordReset(
		ctx context.Context,
		recipient string,
		resetURL string,
	) error
}

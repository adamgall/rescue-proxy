package router

import (
	"fmt"
	"net/http"
	"time"

	"github.com/Rocket-Pool-Rescue-Node/credentials"
	"github.com/Rocket-Pool-Rescue-Node/rescue-proxy/metrics"
	gbp "github.com/Rocket-Rescue-Node/guarded-beacon-proxy"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type auth struct {
	metricsRegistry    *metrics.MetricsRegistry
	authValidityWindow time.Duration
	cm                 *credentials.CredentialManager
}

type authenticationError struct {
	msg        string
	gbpStatus  gbp.AuthenticationStatus
	httpStatus int
	grpcCode   codes.Code
}

func (a *authenticationError) Error() string {
	return "authentication failed, " + a.msg
}

func (a *authenticationError) GRPCError() error {
	return status.Error(a.grpcCode, a.Error())
}

func malformed(err error) *authenticationError {
	return &authenticationError{
		msg:        "malformed credentials: " + err.Error(),
		httpStatus: http.StatusUnauthorized,
		grpcCode:   codes.Unauthenticated,
		gbpStatus:  gbp.Unauthorized,
	}
}

func invalid(err error) *authenticationError {
	return &authenticationError{
		msg:        "invalid credentials: " + err.Error(),
		httpStatus: http.StatusUnauthorized,
		grpcCode:   codes.Unauthenticated,
		gbpStatus:  gbp.Unauthorized,
	}
}

func expired() *authenticationError {
	return &authenticationError{
		msg:        "expired credentials",
		httpStatus: http.StatusUnauthorized,
		grpcCode:   codes.PermissionDenied,
		gbpStatus:  gbp.Forbidden,
	}
}

// authenticate returns nil if the username/password are valid and current
// username/password must be base64url encoded
// otherwise, it returns an authentication error
func (a *auth) authenticate(username, password string) (*credentials.AuthenticatedCredential, *authenticationError) {

	ac := credentials.AuthenticatedCredential{}
	if len(username) == 0 || len(password) == 0 {
		a.metricsRegistry.Counter("malformed").Inc()
		return nil, malformed(fmt.Errorf("username or password missing"))
	}

	err := ac.Base64URLDecode(username, password)
	if err != nil {
		a.metricsRegistry.Counter("malformed").Inc()
		return nil, malformed(err)
	}

	err = a.cm.Verify(&ac)
	if err != nil {
		a.metricsRegistry.Counter("invalid").Inc()
		return nil, invalid(err)
	}

	// Grab the timestamp and make sure the credential is recent enough
	ts := time.Unix(ac.Credential.Timestamp, 0)
	now := time.Now()
	if ts.Before(now) && now.Sub(ts) > a.authValidityWindow {
		a.metricsRegistry.Counter("expired").Inc()
		return nil, expired()
	}

	a.metricsRegistry.Counter("valid").Inc()
	return &ac, nil
}

func initAuth(credentialManager *credentials.CredentialManager, validityWindow time.Duration) *auth {
	out := new(auth)

	out.authValidityWindow = validityWindow
	out.cm = credentialManager
	out.metricsRegistry = metrics.NewMetricsRegistry("authentication")

	return out
}

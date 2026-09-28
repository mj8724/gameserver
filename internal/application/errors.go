package application

// ErrorCode identifies a stable application-level failure without coupling a
// use case to HTTP status codes or transport-specific messages.
type ErrorCode string

const (
	CodeAdminPasswordNotConfigured ErrorCode = "admin_password_not_configured"
	CodeInvalidAdminPassword       ErrorCode = "invalid_admin_password"
	CodeUnauthenticated            ErrorCode = "unauthenticated"

	CodeInstallAlreadyRunning ErrorCode = "install_already_running"
	CodeInstallWhileRunning   ErrorCode = "install_while_running"
	CodeServerInstalling      ErrorCode = "server_installing"
	CodeServerNotInstalled    ErrorCode = "server_not_installed"
	CodeStartFailed           ErrorCode = "start_failed"
	CodeStopFailed            ErrorCode = "stop_failed"
	CodeRestartStopFailed     ErrorCode = "restart_stop_failed"
	CodeRestartNotInstalled   ErrorCode = "restart_not_installed"
	CodeRestartFailed         ErrorCode = "restart_failed"
	CodeServerNotRunning      ErrorCode = "server_not_running"
	CodeValidation            ErrorCode = "validation"
	CodeWorkshopIDNotNumeric  ErrorCode = "workshop_id_not_numeric"
	CodeRenewalOutOfRange     ErrorCode = "renewal_out_of_range"
	CodeInstanceOwned         ErrorCode = "instance_owned_by_another_process"
	CodeRecoveryRequired      ErrorCode = "recovery_required"
	CodeOperationFailed       ErrorCode = "operation_failed"
)

// UseCaseError carries a stable code and, when safe, a public validation
// message. Cause is retained for internal diagnostics but is deliberately not
// included by Error so wrapped infrastructure errors cannot leak secrets over
// a transport.
type UseCaseError struct {
	Code    ErrorCode
	Message string
	Cause   error
}

// Error returns a non-sensitive description of the failure.
func (e *UseCaseError) Error() string {
	if e == nil {
		return "application error"
	}
	if e.Message != "" {
		return e.Message
	}
	return string(e.Code)
}

// Unwrap exposes the underlying error to trusted application diagnostics.
func (e *UseCaseError) Unwrap() error { return e.Cause }

// NewError constructs a typed application error.
func NewError(code ErrorCode, message string) *UseCaseError {
	return &UseCaseError{Code: code, Message: message}
}

// WrapError constructs a typed application error with a private cause.
func WrapError(code ErrorCode, message string, cause error) *UseCaseError {
	return &UseCaseError{Code: code, Message: message, Cause: cause}
}

// ErrorCodeOf returns the typed use-case code, if err wraps one.
func ErrorCodeOf(err error) (ErrorCode, bool) {
	for err != nil {
		if typed, ok := err.(*UseCaseError); ok {
			return typed.Code, true
		}
		unwrapper, ok := err.(interface{ Unwrap() error })
		if !ok {
			break
		}
		err = unwrapper.Unwrap()
	}
	return "", false
}

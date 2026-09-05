package gamev1

// TokenHeader carries the shared secret that authenticates the admin panel to
// the tmServer's control API.
//
// It lives beside the generated code rather than in either service because both
// sides need it and neither may import the other: Go's internal rule keeps the
// adminServer out of tmserver/internal, and a second copy of the string would be
// free to drift — the failure being an endpoint that rejects every call with no
// hint as to why.
//
// Lower-case because gRPC normalises metadata keys that way; a capitalised
// constant would silently never match.
const TokenHeader = "x-w2pp-control-token"

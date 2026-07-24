// Package authz implements the server-side authorization policy for
// PVE-Longxin64.
//
// This package deliberately performs authorization only. It does not
// authenticate users, parse HTTP headers, trust caller-provided role names, or
// load resources. A verified authentication middleware must construct and
// inject Principal values, and service code must build ResourceScope values
// from trusted server-side data before calling Authorize or Allowed.
//
// The policy is default-deny: unknown roles, unknown permissions, incomplete
// ownership data, and cross-organization access are rejected. The sole
// cross-organization exception is the explicit platform administrator role.
package authz

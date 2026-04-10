// Package session provides a public API for speculative code editing:
// applying LSP-validated edits in memory, evaluating their diagnostic
// impact, and committing or discarding the result.
//
// The primary type is [SessionManager], which manages the lifecycle of
// simulation sessions. A session is a transactional unit of work:
//
//  1. Create a session with [SessionManager.CreateSession].
//  2. Apply in-memory edits with [SessionManager.ApplyEdit] or a chain of
//     edits with [SessionManager.SimulateChain].
//  3. Evaluate the net diagnostic delta with [SessionManager.Evaluate].
//  4. Either commit the edits to disk with [SessionManager.Commit] or
//     revert with [SessionManager.Discard].
//
// [SessionStatus] tracks the lifecycle state of each session.
// [EvaluationResult] reports introduced and resolved diagnostics relative to
// a baseline snapshot captured at the start of each file's first edit.
//
// All types in this package are type aliases of the internal implementation;
// values are interchangeable with internal/session without conversion.
package session

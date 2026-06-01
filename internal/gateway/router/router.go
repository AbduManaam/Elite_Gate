package router

// Static fallback routes removed.
// All routing is now driven dynamically from the database
// via the runtime.Loader snapshot (upstreams.target_url joined to routes).
// This file is kept for the MatchHTTP and MatchGRPC functions only.
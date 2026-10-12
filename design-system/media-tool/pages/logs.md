# Logs console override

Reference: user-provided system-log screenshot (2026-10-12).

- White title/actions/filter area above a dark, bounded scrolling console.
- Keyword, severity, genuine module and time-range filters; show latest records
  first. Defaults to 200 per page, supporting 50/100/200. Matching total comes
  from the backend's bounded scan, not the displayed page or fabricated data.
- Monospaced compact rows: local time, textual colored severity, message,
  structured fields and real source file:line when available. Legacy rows do not
  fabricate module paths, trace IDs or stack traces.
- Clear mode hides supplementary fields; per-row keyboard-accessible buttons
  expand complete formatted fields. Source capture strips deployment paths.
- Refresh, confirmed file-log cleanup, copy/download **current redacted page**,
  and a settings drawer for retained file sizes / runtime log level.
- No public share/upload, AI debugging button or fake stack-trace capability.
  HTTP clipboard restrictions use an explicit read-only text dialog instead of
  silently failing or uploading data.
- On mobile filters wrap and rows display time/severity above wrapping messages;
  console contains its own scroll area. Keep focus visible and honor reduced
  motion; no page-level horizontal overflow.
- Store query scans at most each file's final 2 MiB and 20000 records. A returned
  cutoff excludes later records while paging; large writes, rotation or cleanup
  can still move the scan window. The UI explicitly says this is a recent-log
  window, not all retained logs, and exposes truncation.

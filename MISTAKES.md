# Mistakes

- 2026-07-07: I chained a read-only inspection command with `&&`, despite the
  repo workflow asking agents to avoid command separators because they make
  output noisier. Use separate tool calls or `multi_tool_use.parallel` for
  independent reads.

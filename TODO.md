# TODO items

- [x] (Highest priority) Add support for per-category files storage (e.g. `avoidance.yaml`, `security.yaml`, etc.)
- [x] (High priority) Add support for command-based daemon behavior like jimschubert/hi, as it's simpler to configure and use across multiple agents
- [ ] Support lazy loading of category files
- [ ] Support sqlite for storage (will require import/export commands)
- [X] Add support for embeddings
- [X] Add support for semantic search
- [X] Use semantic search to avoid inserting duplicates
- [X] Add support for vector databases (maybe NornicDB) -- sqlite3 with vector extension
- [X] Add support for HNSW indexes
- [X] Lint rewording (e.g. caveman?))
- [ ] Lint YOLO command to automatically merge by some rule
  - e.g. keep the entry with fewer tokens
  - should warn on <0.85 similarity ("Make sure your store is committed before proceeding")
- [X] update stop command with a `--broadcast` to shutdown daemon and any attached http servers -- added stdio like polling
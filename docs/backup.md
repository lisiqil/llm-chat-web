# 备份与恢复

数据库是 PostgreSQL 16，连接串放在 `.env` 的 `DATABASE_URL`。不要把这份连接串和 API Key 打进备份包。

## 备份

```bash
pg_dump --format=custom --file "backups/llmAppDb.dump" "$DATABASE_URL"
```

建议每次升级前备份。删除会话会物理删除正文，但旧备份里仍可能有这些内容。

## 恢复

1. 停止后端进程。
2. 用 `pg_restore` 恢复到空库，或按备份时的库名恢复。
3. 如果备份比当前代码更旧，再执行 `python -m app.cli upgrade`。
4. 启动后，原先处于 `running` 或 `cancel_requested` 的任务会被标记为 `interrupted`，需要用户手动重试。

迁移失败时，用升级前的备份恢复，不要继续在半成品结构上写数据。

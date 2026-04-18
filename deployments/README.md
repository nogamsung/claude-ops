# Deployment Guide — scheduled-dev-agent

## Prerequisites

All deployments require the following on the host machine:

1. **Claude Code CLI** — installed and logged in (`claude login`)
2. **GitHub CLI** — authenticated (`gh auth login`)
3. **git** 2.17+ (for `git worktree` support)

## systemd Deployment

```bash
# 1. Build binary
make build
sudo cp bin/scheduled-dev-agent /usr/local/bin/

# 2. Create config directory
sudo mkdir -p /etc/scheduled-dev-agent /srv/scheduled-dev-agent/data /srv/scheduled-dev-agent/.worktrees

# 3. Copy and edit config
sudo cp config.example.yaml /etc/scheduled-dev-agent/config.yaml
sudo vim /etc/scheduled-dev-agent/config.yaml

# 4. Create .env with secrets (chmod 600)
sudo bash -c 'cat > /etc/scheduled-dev-agent/.env' <<EOF
GITHUB_TOKEN=ghp_your_token_here
SLACK_BOT_TOKEN=xoxb_your_token_here
SLACK_SIGNING_SECRET=your_secret_here
EOF
sudo chmod 600 /etc/scheduled-dev-agent/.env

# 5. Edit and install systemd unit
sudo cp deployments/scheduled-dev-agent.service /etc/systemd/system/
# Edit User= and Group= in the unit file to match the operator user
sudo vim /etc/systemd/system/scheduled-dev-agent.service

# 6. Enable and start
sudo systemctl daemon-reload
sudo systemctl enable scheduled-dev-agent
sudo systemctl start scheduled-dev-agent
sudo systemctl status scheduled-dev-agent
```

## Docker Deployment

```bash
# 1. Copy and edit .env
cp .env.example .env
vim .env  # Fill in GITHUB_TOKEN, SLACK_BOT_TOKEN, SLACK_SIGNING_SECRET

# 2. Start
docker-compose -f deployments/docker-compose.yml up -d

# 3. Check logs
docker-compose -f deployments/docker-compose.yml logs -f
```

**Important Docker Notes:**

- The `claude` and `gh` binaries on the host are bind-mounted read-only into the container.
- `~/.claude` (session files) is mounted read-only. The operator must run `claude login` on the host **before** starting the container.
- `~/.config/gh` is mounted read-only for GitHub CLI auth.
- Worktrees at `.worktrees/` must be RW — Claude modifies files there.

## Slack Configuration

1. Create a Slack app at https://api.slack.com/apps
2. Grant scopes: `chat:write`, `chat:write.public`
3. Enable Interactivity and set the request URL to: `https://your-server/slack/interactions`
4. Copy the Signing Secret to `SLACK_SIGNING_SECRET`
5. Install the app to your workspace and copy the Bot Token to `SLACK_BOT_TOKEN`

## Security Notes (PRD §9)

- GitHub PAT, Slack bot token, and signing secret must **only** be set via environment variables — never in `config.yaml`
- `~/.claude` session directory should have permissions `0700` on the host
- The HTTP server binds to `127.0.0.1:8787` by default — only expose the Slack interactions endpoint via a reverse proxy (nginx/caddy)
- Slack signing secret verification uses 5-minute replay protection (HMAC-SHA256)

#!/bin/bash

# -------------------------
# 기본 설정
# -------------------------
APP_NAME="claude-ops"
OUTER_PORT=8787
INNER_PORT=8787
IMAGE="ghcr.io/nogamsung/claude-ops:latest"

ENV_FILE="/home/nogamsung/app/env/claude-ops"
CONFIG_FILE="/home/nogamsung/app/claude-ops/config.yaml"
DATA_DIR="/home/nogamsung/app/claude-ops/data"
WORKTREE_DIR="/home/nogamsung/app/claude-ops/.worktrees"
PROMPTS_DIR="/home/nogamsung/app/claude-ops/prompts"
LOG_DIR="/home/nogamsung/app/claude-ops/logs"

# Host 에 이미 'claude login' / 'gh auth login' 이 완료돼 있어야 함 (PRD §9).
# 컨테이너는 agent 유저로 구동되지만 HOME 은 /root 로 고정되므로 /root 에 마운트.
CLAUDE_SESSION_DIR="$HOME/.claude"
GH_CONFIG_DIR="$HOME/.config/gh"
GIT_CONFIG="$HOME/.gitconfig"
CLAUDE_BIN="/usr/local/bin/claude"
GH_BIN="/usr/local/bin/gh"

# -------------------------
# 이미지 pull (최신 latest 확보)
# -------------------------
sudo docker pull $IMAGE

# -------------------------
# 이전 컨테이너 종료
# -------------------------
sudo docker kill $APP_NAME
sudo docker rm $APP_NAME

# -------------------------
# 새 컨테이너 실행
# -------------------------
# - HTTP 는 localhost 에만 바인딩 (Slack interactions 은 nginx/caddy 리버스 프록시로 노출)
# - /config, /prompts, claude / gh 세션은 read-only
# - /data, /worktrees, /logs 는 read-write (Claude 가 파일 수정)
docker run -itd \
    --name $APP_NAME \
    --restart=always \
    --env-file $ENV_FILE \
    -p 127.0.0.1:$OUTER_PORT:$INNER_PORT \
    -v $CONFIG_FILE:/config/config.yaml:ro \
    -v $DATA_DIR:/data \
    -v $WORKTREE_DIR:/worktrees \
    -v $PROMPTS_DIR:/prompts:ro \
    -v $LOG_DIR:/logs \
    -v $CLAUDE_SESSION_DIR:/root/.claude:ro \
    -v $GH_CONFIG_DIR:/root/.config/gh:ro \
    -v $GIT_CONFIG:/root/.gitconfig:ro \
    -v $CLAUDE_BIN:/usr/local/bin/claude:ro \
    -v $GH_BIN:/usr/local/bin/gh:ro \
    $IMAGE \
    -config /config/config.yaml

# -------------------------
# 새 애플리케이션 로그 확인
# -------------------------
sudo docker logs -f $APP_NAME

# Template: linux-onstart-claude

A Linux machine that, on start, listens for Robot Dreams messages and hands
each one to an AI coding agent running in a folder you control. The agent
reads the message, does what it says, and reports back over the same message
channel.

This brief installs nothing. Every file below is yours to create, read
first, and change. Nothing starts until you enable the unit at the end.

Read `dream node onboard` first if you have not: it explains the message
types, the storage library, and the credential model this builds on.

## What you are agreeing to

The message channel becomes an instruction channel for this machine.
Anything that can send this node a message can put text in front of an agent
running here, with whatever access that agent has. That is the point of the
template, not a flaw in it — but decide it deliberately rather than discover
it later.

Two narrowings are in the listener below as commented-out lines: accept only
from your org-chart parent, or accept only one message type. Neither is
switched on, because a template is a starting point and only you know what
this node is for.

Handling is at-least-once. The listener acks a message after the agent
finishes, so a crash in between means the message is delivered again and the
work is repeated. Losing an ack costs you a repeat; acking too early costs
you the work.

## The folder

One folder, reused for every message:

  mkdir -p ~/dream-work

It holds `CLAUDE.md`, and `message.json` — the message currently being
handled, rewritten each time.

Because it is one shared folder: two messages arriving at once will trip over
each other, and files left by the previous task are visible to the next. That
is fine for a node doing one thing at a time and worth changing if this gets
busy.

## The instructions the agent reads

  cat > ~/dream-work/CLAUDE.md <<'EOF'
  # You are a Robot Dreams node

  A message arrived for you. It is in ./message.json, one JSON object:

    id            the message id
    type          status_update | completed_work | escalation | request_for_input
    from          the node that sent it
    to            you — this node's id
    subject       a short human line
    body          the payload, as JSON
    storage_ptr   set when the real payload is in the library, not inline

  Do this:

  1. Read ./message.json. If `storage_ptr` is set, the message is a catalog
     card and the book is in the library — fetch it:

       dream storage get "$(jq -r '.storage_ptr.Path' message.json)" -

     Messages carry pointers, never payloads. Do not expect large input in
     `body`.

  2. Do what the message asks, here in this folder.

  3. Report back. Put the result in the library first, then send the message
     that points at it — put before send, never the other way around:

       dream storage put "shared/$(jq -r '.to' message.json)/result-$(date -u +%Y%m%dT%H%M%SZ).md" - < result.md
       dream message send --type completed_work \
         --causation-id "$(jq -r '.id' message.json)" \
         --subject "what you did, in one line" \
         --storage-path "<the path you just wrote>"

     A short result can go inline instead, as JSON:

       dream message send --type completed_work \
         --causation-id "$(jq -r '.id' message.json)" \
         --subject "done" --body '{"ok":true}'

  4. If you cannot do it, say so rather than going quiet:

       dream message send --type escalation \
         --causation-id "$(jq -r '.id' message.json)" \
         --subject "blocked: <why>" --body '{"reason":"<what stopped you>"}'

  Do not ack the message. The listener that started you does that when you
  exit, so that a crash re-runs the work instead of losing it.

  You may write anywhere under shared/ and under workers/<your-node-id>/.
  EOF

## The listener

  mkdir -p ~/.dream/linux-onstart-claude
  cat > ~/.dream/linux-onstart-claude/listen.sh <<'EOF'
  #!/usr/bin/env bash
  # Hand every arriving Robot Dreams message to an AI agent.
  set -uo pipefail

  WORK="${DREAM_WORK:-$HOME/dream-work}"

  # --json is what makes this loop possible: the human line does not carry
  # the message id, and without the id you cannot ack what you handled.
  # Unacked messages come back on every reconnect, so a listener that never
  # acks re-runs its whole history each restart.
  dream message tail --follow --json | while IFS= read -r env; do
    id=$(printf '%s' "$env" | jq -r '.id')
    from=$(printf '%s' "$env" | jq -r '.from')
    type=$(printf '%s' "$env" | jq -r '.type')
    [ -n "$id" ] || continue

    # Narrow what this node will act on, if you want to:
    # [ "$from" = "your-parent-node-id" ] || { dream message ack "$id" --action ignored; continue; }
    # [ "$type" = "request_for_input" ]   || { dream message ack "$id" --action ignored; continue; }

    printf '%s\n' "$env" > "$WORK/message.json"
    ( cd "$WORK" && claude -p "A message arrived. Read message.json and follow CLAUDE.md." )

    # After the agent exits, not before: acking first would lose the work
    # if the agent died.
    dream message ack "$id" --action handled
  done
  EOF
  chmod +x ~/.dream/linux-onstart-claude/listen.sh

The `claude -p ...` line is the one to adjust. It is an interface this
template does not own, and your agent may want different flags, a different
tool, or no AI at all.

## Starting it on boot

Two ways. A user unit needs no root and reads the identity key as the user
who owns it, which is almost always what you want. A system unit starts
without anyone logging in.

Both restart on failure on purpose: the listener exits if the control plane
is not reachable when it starts, so at boot the unit is what retries.

Both pin the server explicitly, because a machine connected to more than one
control plane cannot resolve a bare command.

### A user unit

  cat > ~/.config/systemd/user/dream-node.service <<EOF
  [Unit]
  Description=Robot Dreams node: hand each message to an AI agent
  After=network-online.target

  [Service]
  Environment=DREAM_URL=$DREAM_URL
  ExecStart=%h/.dream/linux-onstart-claude/listen.sh
  Restart=always
  RestartSec=5

  [Install]
  WantedBy=default.target
  EOF

  systemctl --user daemon-reload
  systemctl --user enable --now dream-node

A user unit only runs while you are logged in, unless you allow it to
linger:

  loginctl enable-linger "$USER"

### A system unit

Needs root, and must be told which user's `~/.dream` to use:

  sudo tee /etc/systemd/system/dream-node.service > /dev/null <<EOF
  [Unit]
  Description=Robot Dreams node: hand each message to an AI agent
  After=network-online.target

  [Service]
  User=$USER
  Environment=DREAM_URL=$DREAM_URL
  Environment=PATH=/usr/local/bin:/usr/bin:/bin:$HOME/.local/bin
  ExecStart=$HOME/.dream/linux-onstart-claude/listen.sh
  Restart=always
  RestartSec=5

  [Install]
  WantedBy=multi-user.target
  EOF

  sudo systemctl daemon-reload
  sudo systemctl enable --now dream-node

If `dream`, `claude` or `jq` are not on that PATH, give the unit the paths
they are on — a system unit does not inherit your shell environment.

## Checking it

Watch the listener:

  systemctl --user status dream-node
  journalctl --user -u dream-node -f

Then, from anywhere that can reach the control plane, send this node
something to do:

  dream message send --type request_for_input --to <this-node-id> \
    --subject "say hello" --body '{"task":"write hello.txt in the work folder"}'

You should see `message.json` appear in ~/dream-work, the agent run, and a
`completed_work` message come back. Restart the unit and confirm the same
message is not handled twice — that is what the ack is for.

───────────────────────────────────────────
Start here:
  1. mkdir -p ~/dream-work, and write the CLAUDE.md above.
  2. Write the listener, and chmod +x it.
  3. Enable one of the two units.
Then send this node a message and watch the folder.
───────────────────────────────────────────

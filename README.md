# Standblatt print agent

The print agent is a small program on a computer next to the printers of a shooting range. It only makes outgoing
HTTPS requests to [Standblatt](https://github.com/standblatt/server): it reports the local printers and their
options, waits for print jobs (long polling) and prints them. No port needs to be opened at the range.

## Linux (CUPS)

1. In Standblatt, **Ranges → Printing → Add print agent**; copy the token (shown only once).
2. Download `standblatt-print-agent-linux-amd64` or `-arm64` from the
   [latest release](https://github.com/standblatt/print-agent/releases/latest) and install it as a systemd service:

    ```sh
    install standblatt-print-agent-linux-arm64 /usr/local/bin/standblatt-print-agent
    printf 'STANDBLATT_URL=https://app.standblatt.ch\nSTANDBLATT_AGENT_TOKEN=<token>\n' > /etc/standblatt-print-agent.env
    chmod 600 /etc/standblatt-print-agent.env
    cat > /etc/systemd/system/standblatt-print-agent.service <<'EOF'
    [Unit]
    Description=Standblatt print agent
    After=network-online.target cups.service
    Wants=network-online.target

    [Service]
    EnvironmentFile=/etc/standblatt-print-agent.env
    ExecStart=/usr/local/bin/standblatt-print-agent
    Restart=always
    RestartSec=5
    DynamicUser=yes

    [Install]
    WantedBy=multi-user.target
    EOF
    systemctl enable --now standblatt-print-agent
    ```

3. Back in Standblatt, the agent shows as *online* with its printers. Assign a printer and its options to each
   document.

`journalctl -u standblatt-print-agent` shows the log.

## Configuration

| Variable | Description |
|---|---|
| `STANDBLATT_URL` | base URL of Standblatt, e.g. `https://app.standblatt.ch` |
| `STANDBLATT_AGENT_TOKEN` | token from *Add print agent*; several tokens separated by commas serve several ranges from one computer |

## Behaviour

- Printers and their options come from `lpstat -e` and `lpoptions -p <printer> -l`; documents are printed with
  `lp -d <printer> -t <title> -o <options>`, the options set in the printing setup of the range.
- The agent reports its printers at start and every five minutes. A job that is not confirmed within two minutes goes
  back to the queue; after three attempts it fails.
- Standblatt treats an agent as online when it was seen within the last minute. Revoking the agent in Standblatt
  invalidates its token immediately.

## Development

```sh
make build    # bin/: the agent for this computer and the Linux release binaries
make prepare  # gofmt, go vet, staticcheck, doc comments; before every commit
make test     # Go tests and the contract test against the server image of the CI (needs Docker)
make clean
```

## License

[AGPL-3.0](LICENSE)

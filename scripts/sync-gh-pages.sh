#!/usr/bin/env bash
# Sincroniza a telemetria do Leveling Bot da VPS para o GitHub Pages (docs/status.json)
# MODO STEALTH: nicks da party, e-mails e IPs sÃ£o anonimizados com jq ANTES do push.
set -e

STATUS=$(curl -s http://127.0.0.1:3003/api/status)
if [ -z "$STATUS" ] || [ "$STATUS" = "null" ]; then
  exit 0
fi

STATUS=$(echo "$STATUS" | jq '
  (if (.accounts | type) == "array" then
    .accounts |= (to_entries | map(
        .key as $i
        | .value
        | .id = ("acc-" + (($i + 1) | tostring))
        | .name = ("Party #" + (($i + 1) | tostring))
        | if (.logs | type) == "array" then
            .logs |= map(gsub("(?i)kbelludoo[0-9]*|lohmoreira|lrn"; ("Party #" + (($i + 1) | tostring))))
          else . end
      ))
   else . end)
  | walk(
      if type == "string" and (test("^https?://") | not) then
        gsub("[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}"; "usuario@privado.com")
        | gsub("\\b[0-9]{1,3}(\\.[0-9]{1,3}){3}\\b"; "x.x.x.x")
        | gsub("(?i)kbelludoo[0-9]*|lohmoreira|lrn"; "treinador")
      else . end
    )
')

if [ -z "$STATUS" ] || [ "$STATUS" = "null" ]; then
  exit 0
fi

B64=$(echo "$STATUS" | base64 -w 0)
SHA=$(gh api repos/kbelludoo/leveling-bot/contents/docs/status.json --jq .sha 2>/dev/null || true)

if [ -n "$SHA" ]; then
  gh api -X PUT repos/kbelludoo/leveling-bot/contents/docs/status.json \
    -f message="telemetry: live bot status update (stealth)" \
    -f content="$B64" \
    -f sha="$SHA" > /dev/null 2>&1
else
  gh api -X PUT repos/kbelludoo/leveling-bot/contents/docs/status.json \
    -f message="telemetry: live bot status update (stealth)" \
    -f content="$B64" > /dev/null 2>&1
fi
echo "Telemetria Leveling Bot anonimizada sincronizada com o GitHub Pages!"

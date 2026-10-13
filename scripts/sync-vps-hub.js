#!/usr/bin/env node
const http = require('http');
const { execSync } = require('child_process');
// MODO STEALTH: scrub profundo de todo o payload antes do push (nicks, e-mails, IPs).
const NICK_RE = /kbelludoo[0-9]*|lohmoreira|\blrn\b/gi;
const EMAIL_RE = /[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}/g;
const IP_RE = /\b\d{1,3}(?:\.\d{1,3}){3}\b/g;

function stealthScrub(value) {
  if (typeof value === 'string') {
    if (/^https?:\/\//i.test(value)) return value;
    return value
      .replace(EMAIL_RE, 'usuario@privado.com')
      .replace(IP_RE, 'x.x.x.x')
      .replace(NICK_RE, 'treinador');
  }
  if (Array.isArray(value)) return value.map(stealthScrub);
  if (value && typeof value === 'object') {
    const out = {};
    let n = 0;
    for (const rawKey of Object.keys(value)) {
      let key = rawKey;
      if (!/^https?:\/\//i.test(rawKey)) key = rawKey.replace(NICK_RE, 'treinador');
      while (Object.prototype.hasOwnProperty.call(out, key)) key = `${key}_${++n}`;
      out[key] = stealthScrub(value[rawKey]);
    }
    return out;
  }
  return value;
}

function fetchJson(url) {
  return new Promise((resolve) => {
    const req = http.get(url, { timeout: 2500 }, (res) => {
      let data = '';
      res.on('data', chunk => data += chunk);
      res.on('end', () => {
        try {
          const parsed = JSON.parse(data);
          resolve({ online: true, ...parsed });
        } catch (e) {
          resolve({ online: false, error: 'JSON inválido' });
        }
      });
    });
    req.on('error', (err) => resolve({ online: false, error: err.message }));
    req.on('timeout', () => {
      req.destroy();
      resolve({ online: false, error: 'Timeout de conexão' });
    });
  });
}

function getSystemMetrics() {
  try {
    const uptime = execSync('uptime -p 2>/dev/null || uptime').toString().trim();
    const loadParts = execSync('cat /proc/loadavg 2>/dev/null || echo "0 0 0"').toString().trim().split(/\s+/);
    const freeRaw = execSync('free -m 2>/dev/null').toString();
    
    let ramTotal = 0, ramUsed = 0, ramFree = 0, ramAvail = 0;
    let swapTotal = 0, swapUsed = 0;

    for (const line of freeRaw.split('\n')) {
      const parts = line.trim().split(/\s+/);
      if (parts[0] === 'Mem:') {
        ramTotal = parseInt(parts[1], 10) || 0;
        ramUsed = parseInt(parts[2], 10) || 0;
        ramFree = parseInt(parts[3], 10) || 0;
        ramAvail = parseInt(parts[6] || parts[3], 10) || 0;
      } else if (parts[0] === 'Swap:') {
        swapTotal = parseInt(parts[1], 10) || 0;
        swapUsed = parseInt(parts[2], 10) || 0;
      }
    }

    return {
      uptime,
      load: loadParts.slice(0, 3).join(', '),
      ramTotalMb: ramTotal,
      ramUsedMb: ramUsed,
      ramAvailableMb: ramAvail,
      ramPercent: ramTotal ? Math.round((ramUsed / ramTotal) * 100) : 0,
      swapTotalMb: swapTotal,
      swapUsedMb: swapUsed
    };
  } catch (err) {
    return {
      uptime: 'N/A',
      load: 'N/A',
      ramTotalMb: 1024,
      ramUsedMb: 512,
      ramAvailableMb: 512,
      ramPercent: 50,
      swapTotalMb: 2048,
      swapUsedMb: 0
    };
  }
}

async function main() {
  const vps = getSystemMetrics();

  const [huntera, tibidle, sigma] = await Promise.all([
    fetchJson('http://127.0.0.1:3000/api/status'),
    fetchJson('http://127.0.0.1:3001/api/status'),
    fetchJson('http://127.0.0.1:3002/api/status')
  ]);

  const bots = {
    huntera: {
      name: 'Huntera Idle Bot',
      game: 'Huntera',
      port: 3000,
      dashboardUrl: 'https://kbelludoo.github.io/huntera-bot/',
      status: huntera
    },
    tibidle: {
      name: 'Tibidle Multi-Bot',
      game: 'Tibidle',
      port: 3001,
      dashboardUrl: 'https://kbelludoo.github.io/tibidle-bot/',
      status: tibidle
    },
    sigma: {
      name: 'Pokemon Sigma Multi-Bot',
      game: 'Pokemon Sigma',
      port: 3002,
      dashboardUrl: 'https://kbelludoo.github.io/sigma-bot/',
      status: sigma
    }
  };

  let activeBotsCount = 0;
  if (huntera.online) activeBotsCount++;
  if (tibidle.online) activeBotsCount++;
  if (sigma.online) activeBotsCount++;

  const payload = {
    timestamp: new Date().toISOString(),
    vps,
    summary: {
      totalBots: 3,
      activeBots: activeBotsCount
    },
    bots
  };

  const jsonStr = JSON.stringify(stealthScrub(payload), null, 2);

  // Se executado com push (--push) ou standalone no crontab
  if (process.argv.includes('--push')) {
    try {
      const b64 = Buffer.from(jsonStr).toString('base64');
      let sha = '';
      try {
        sha = execSync('gh api repos/kbelludoo/bot-template-go/contents/docs/status.json --jq .sha 2>/dev/null')
          .toString().trim();
      } catch (e) {
        // Arquivo ainda não existe
      }

      if (sha) {
        execSync(`gh api -X PUT repos/kbelludoo/bot-template-go/contents/docs/status.json -f message="telemetry: live vps bots summary" -f content="${b64}" -f sha="${sha}" > /dev/null 2>&1`);
      } else {
        execSync(`gh api -X PUT repos/kbelludoo/bot-template-go/contents/docs/status.json -f message="telemetry: live vps bots summary" -f content="${b64}" > /dev/null 2>&1`);
      }
      console.log('✓ Resumo da VPS sincronizado com sucesso no GitHub Pages!');
    } catch (pushErr) {
      console.error('[-] Falha ao sincronizar via GitHub API:', pushErr.message);
    }
  } else {
    process.stdout.write(jsonStr);
  }
}

main();

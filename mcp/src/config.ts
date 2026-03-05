export interface Config {
  // NFC Agent
  agentUrl: string;

  // Dev tools (optional)
  repoPath: string | undefined;

  // SimplyPrint (optional)
  simplyPrintApiKey: string | undefined;
  simplyPrintBaseUrl: string;

  // whatt.io (optional)
  whattioToken: string | undefined;
  whattioTeamId: string | undefined;

  // Feature flags
  hasDevTools: boolean;
  hasSimplyPrint: boolean;
  hasWhattio: boolean;
}

export function loadConfig(): Config {
  const agentUrl = process.env.NFC_AGENT_URL ?? 'http://127.0.0.1:32145';
  const repoPath = process.env.NFC_AGENT_REPO_PATH || undefined;
  const simplyPrintApiKey = process.env.SIMPLYPRINT_API_KEY || undefined;
  const simplyPrintBaseUrl = process.env.SIMPLYPRINT_BASE_URL ?? 'https://api.simplyprint.io/0';
  const whattioToken = process.env.WHATTIO_TOKEN || undefined;
  const whattioTeamId = process.env.WHATTIO_TEAM_ID || undefined;

  return {
    agentUrl,
    repoPath,
    simplyPrintApiKey,
    simplyPrintBaseUrl,
    whattioToken,
    whattioTeamId,
    hasDevTools: repoPath !== undefined,
    hasSimplyPrint: simplyPrintApiKey !== undefined,
    hasWhattio: whattioToken !== undefined,
  };
}

import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';

export interface SetupState {
  version: string;
  timestamp: string;
  currentState: string;
  completedSteps: string[];
  context: {
    mcpInstallPath?: string;
    chezmoiDetected?: boolean;
    workMachine?: boolean;
    credentialsPath?: string;
    tokenPath?: string;
    selectedAgents?: string[];
    selectedMcps?: string[];
    slackBotToken?: string;
    slackTeamId?: string;
  };
}

const STATE_FILE_PATH = path.join(os.homedir(), '.mcp-wizard-state.json');

export async function saveState(state: SetupState): Promise<void> {
  await fs.writeFile(STATE_FILE_PATH, JSON.stringify(state, null, 2));
}

export async function loadState(): Promise<SetupState | null> {
  try {
    await fs.access(STATE_FILE_PATH);
    const data = await fs.readFile(STATE_FILE_PATH, 'utf-8');
    return JSON.parse(data);
  } catch {
    return null;
  }
}

export async function clearState(): Promise<void> {
  try {
    await fs.unlink(STATE_FILE_PATH);
  } catch {
    // File doesn't exist, that's fine
  }
}

export function createNewState(): SetupState {
  return {
    version: '1.0.0',
    timestamp: new Date().toISOString(),
    currentState: 'START',
    completedSteps: [],
    context: {},
  };
}

export function updateState(
  state: SetupState,
  currentState: string,
  newStep?: string
): SetupState {
  if (newStep && !state.completedSteps.includes(newStep)) {
    state.completedSteps.push(newStep);
  }
  state.currentState = currentState;
  state.timestamp = new Date().toISOString();
  return state;
}

// Setup flow states (from D3 state machine)
export const SETUP_STATES = {
  START: 'START',
  DETECT_ENVIRONMENT: 'DETECT_ENVIRONMENT',
  CHECK_MCP_INSTALLATION: 'CHECK_MCP_INSTALLATION',
  INSTALL_MCP: 'INSTALL_MCP',
  CHECK_CREDENTIALS: 'CHECK_CREDENTIALS',
  GCP_CONSOLE_GUIDE: 'GCP_CONSOLE_GUIDE',
  UPLOAD_CREDENTIALS: 'UPLOAD_CREDENTIALS',
  OAUTH_FLOW: 'OAUTH_FLOW',
  SAVE_TOKEN: 'SAVE_TOKEN',
  UPDATE_CONFIG: 'UPDATE_CONFIG',
  SHOW_CHEZMOI_SNIPPET: 'SHOW_CHEZMOI_SNIPPET',
  WRITE_MCP_CONFIG: 'WRITE_MCP_CONFIG',
  VERIFY_SETUP: 'VERIFY_SETUP',
  SUCCESS: 'SUCCESS',
  ERROR: 'ERROR',
  CANCELLED: 'CANCELLED',
} as const;

export type SetupStateType = (typeof SETUP_STATES)[keyof typeof SETUP_STATES];

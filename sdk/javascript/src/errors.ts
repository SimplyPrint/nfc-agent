/**
 * Base error class for NFC Agent SDK errors
 */
export class NFCAgentError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'NFCAgentError';
    Object.setPrototypeOf(this, new.target.prototype);
  }
}

/**
 * Error thrown when connection to the nfc-agent server fails
 */
export class ConnectionError extends NFCAgentError {
  constructor(message: string = 'Failed to connect to nfc-agent server') {
    super(message);
    this.name = 'ConnectionError';
  }
}

/**
 * Error thrown for reader-related issues
 */
export class ReaderError extends NFCAgentError {
  constructor(message: string) {
    super(message);
    this.name = 'ReaderError';
  }
}

/**
 * Error thrown for card-related issues (read/write failures, no card present)
 */
export class CardError extends NFCAgentError {
  constructor(message: string) {
    super(message);
    this.name = 'CardError';
  }
}

/**
 * Error thrown when the API returns an error response
 */
export class APIError extends NFCAgentError {
  public readonly statusCode: number;

  constructor(message: string, statusCode: number) {
    super(message);
    this.name = 'APIError';
    this.statusCode = statusCode;
  }
}

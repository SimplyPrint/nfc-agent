import { describe, it, expect } from 'vitest';
import {
  NFCAgentError,
  ConnectionError,
  ReaderError,
  CardError,
  APIError,
} from './errors.js';

describe('NFCAgentError', () => {
  it('should create an error with the correct message', () => {
    const error = new NFCAgentError('Test error');
    expect(error.message).toBe('Test error');
    expect(error.name).toBe('NFCAgentError');
  });

  it('should be an instance of Error', () => {
    const error = new NFCAgentError('Test');
    expect(error).toBeInstanceOf(Error);
    expect(error).toBeInstanceOf(NFCAgentError);
  });
});

describe('ConnectionError', () => {
  it('should create an error with default message', () => {
    const error = new ConnectionError();
    expect(error.message).toBe('Failed to connect to nfc-agent server');
    expect(error.name).toBe('ConnectionError');
  });

  it('should create an error with custom message', () => {
    const error = new ConnectionError('Custom message');
    expect(error.message).toBe('Custom message');
  });

  it('should be an instance of NFCAgentError', () => {
    const error = new ConnectionError();
    expect(error).toBeInstanceOf(NFCAgentError);
    expect(error).toBeInstanceOf(ConnectionError);
  });
});

describe('ReaderError', () => {
  it('should create an error with the correct message', () => {
    const error = new ReaderError('Reader not found');
    expect(error.message).toBe('Reader not found');
    expect(error.name).toBe('ReaderError');
  });

  it('should be an instance of NFCAgentError', () => {
    const error = new ReaderError('Test');
    expect(error).toBeInstanceOf(NFCAgentError);
  });
});

describe('CardError', () => {
  it('should create an error with the correct message', () => {
    const error = new CardError('No card in field');
    expect(error.message).toBe('No card in field');
    expect(error.name).toBe('CardError');
  });

  it('should be an instance of NFCAgentError', () => {
    const error = new CardError('Test');
    expect(error).toBeInstanceOf(NFCAgentError);
  });
});

describe('APIError', () => {
  it('should create an error with message and status code', () => {
    const error = new APIError('Not found', 404);
    expect(error.message).toBe('Not found');
    expect(error.statusCode).toBe(404);
    expect(error.name).toBe('APIError');
  });

  it('should be an instance of NFCAgentError', () => {
    const error = new APIError('Test', 500);
    expect(error).toBeInstanceOf(NFCAgentError);
  });
});

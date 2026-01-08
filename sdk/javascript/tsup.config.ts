import { defineConfig } from 'tsup';

export default defineConfig({
  entry: ['src/index.ts'],
  format: ['cjs', 'esm', 'iife'],
  dts: true,
  clean: true,
  minify: true,
  sourcemap: true,
  globalName: 'NFCAgent',
  outExtension({ format }) {
    if (format === 'iife') {
      return { js: '.min.js' };
    }
    return {};
  },
});

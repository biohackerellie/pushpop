import { defineConfig } from 'tsup';

export default defineConfig({
  entry: ['src/index.ts'],
  format: ['esm'],
  minify: true,
  target: 'esnext',
  dts: true,
  clean: true,
});

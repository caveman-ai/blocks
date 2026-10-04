# npm packages

`caveman-blocks` is a 20-line shim that resolves a per-platform optional dependency and execs the native
binary (the esbuild and Biome layout). The platform packages are generated at release time from
`platform/package.json.tmpl` by `scripts/publish-npm.sh`, which GoReleaser calls after the binaries are
built. The shim is a bootstrap for `npx caveman-blocks init|scan`; hooks never go through it.

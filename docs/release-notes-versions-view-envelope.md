# Versions view prints Apple's envelope

`asc versions view` and `asc versions list` now keep every `appStoreVersions`
attribute App Store Connect returns. Earlier releases dropped `reviewType` and
`usesIdfa` and removed null attributes such as `"earliestReleaseDate": null`.

`asc versions phased-release view` now prints `"data": null` when the version
has no phased release, instead of an empty `{"type":"","id":""}` resource.
`asc status` reports that phased release as `"configured": false` instead of
configured with an empty ID.

## Deprecated: the `versions view` summary

Without `--include`, `asc versions view` prints a reduced summary (`id`,
`versionString`, `platform`, `state`, `downloadable`, and the
`--include-build`/`--include-submission` IDs) that is not Apple's envelope. It
keeps that shape in this release and, for JSON output, prints this warning to stderr:

```text
Warning: `asc versions view` without --include prints a reduced summary; the next major release prints Apple's appStoreVersions envelope instead. Pass --include to get the envelope now: --include build replaces --include-build, --include appStoreVersionSubmission replaces --include-submission.
```

The next major release prints Apple's envelope by default. Migrate now:

| Before | After |
|---|---|
| `asc versions view --version-id ID --include-build` | `asc versions view --version-id ID --include build` |
| `asc versions view --version-id ID --include-submission` | `asc versions view --version-id ID --include appStoreVersionSubmission` |
| `.versionString`, `.platform`, `.downloadable` | `.data.attributes.versionString`, `.data.attributes.platform`, `.data.attributes.downloadable` |
| `.state` | `.data.attributes.appVersionState` (falls back to `.data.attributes.appStoreState`) |
| `.buildId`, `.buildVersion` | `.data.relationships.build.data.id`, and the `builds` entry in `.included` |
| `.submissionId` | `.data.relationships.appStoreVersionSubmission.data.id` |

# Job-Forker

## Functionality
The `job-forker` scans all the files in the specified directory `dir` for Prow-Job-Configurations with the annotation `fork-per-release` and creates a seperate Prow-Job-Configuration that targets the specific release branches. Those configs should not be updated by the autobumper and are release-specific.
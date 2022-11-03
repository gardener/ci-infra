# Job-Forker

## Functionality
The `job-forker` scans all the files in the specified directory `dir` for Prow-Job-Configurations with the annotation `fork-per-release` and creates a separate Prow-Job-Configuration that targets the specific release branches.

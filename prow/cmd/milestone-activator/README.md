# Milestone-Activator
## Functionality
The `milestone-activator` scans GitHub repositories for milestones of a specific pattern. When a matching milestone was added, it uncomments parts of prow configuration which is tagged for this milestone. When the milestone is comments the respective parts out again. `milestone-activator` commits the resulting file and creates a PR for it. 

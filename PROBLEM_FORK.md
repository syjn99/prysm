# Problem and Feasibility check.

## Problem

- Most of the Prysm codebase is too old.
- We have about >2 forks per year these days, it is quite redundant to have every BeaconState and BeaconBlock.
- Let's debloat the codebase.

## (Maybe) Solution

> Assume that current fork is N.

- Nuke all code below N-1. (Only remaining N-1, N).
- When every new hardfork is coming, bump the version. For example, current v7 is targetting Fulu. If we go Gloas, go v8.
- The node should be able to sync from the genesis tho. Provide a different binary that can handle past version. This binary will import the previous version as a dependency, and for example, run a STF for the past fork with the previous version.
# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/). This project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

# [Unreleased]

## Added

- `dr workload delete --dir <path>`: which project's manifest holds the binding to clear, matching the flag `up` and `config` already take.

## Fixed

- `dr workload delete` now clears a `workloadId` written as a YAML alias. The reader resolves the alias before answering, so such a binding is the one the project deploys to, but the removal compared the unresolved node and left the file as stale as it found it while reporting the delete a success.
- `dr workload up --dir <path>` now says which flag named a path that is not a directory, instead of failing with a bare `not a directory`, matching what `dr workload delete --dir` already said.
- An empty `.datarobot.yaml` now gets one explanation and one remedy whichever command reaches it, rather than a generic "root must be a YAML mapping" from the write path and a friendlier sentence from the read path.

## Changed

- `dr workload delete` now removes the `workloadId` it wrote from a `.datarobot.yaml` bound to the workload it deleted, so the next `dr workload up` is not pointed at something that no longer exists. Only a manifest naming that exact id is touched, the id is compared inside the same edit that removes it, and the artifact link under `.datarobot/` is left alone so the next deploy reuses the artifact; the command names that artifact and how to unlink from it. A workload that was already gone leaves the binding alone: a 404 means "not on this instance", which is not proof the binding is stale.
- `dr workload up` now mints a new artifact when the one this project is linked to no longer says what `.datarobot.yaml` says. An artifact's spec is fixed when it is created, so reusing a diverged one deployed the frozen answer and reported success. A read that fails is not treated as a difference: the run stops and names the artifact it could not read, so a timeout never starts a new artifact lineage.
- `dr workload up` now treats a binding that resolves to nothing as drift and creates a new workload, instead of refusing and asking for the id to be cleared by hand. The plan and the JSON envelope both name the id it could not find, so a run pointed at the wrong instance is visible rather than silent. A terminated workload is still refused, because it continues to exist and to hold its name and artifact; the message now names `dr workload delete`, which clears the way and the binding together.

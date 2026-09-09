### Fixed

- Read the Gloas fork-choice spectest `head.payload_status` check from the head object where consensus-specs v1.7.0-beta.0 emits it. The check silently never ran because it looked for a top-level `head_payload_status` key that the vectors no longer carry.

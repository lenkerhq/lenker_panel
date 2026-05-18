ALTER TABLE nodes DROP COLUMN xray_pid;
ALTER TABLE nodes DROP CONSTRAINT IF EXISTS nodes_runtime_process_state_check;
ALTER TABLE nodes ADD CONSTRAINT nodes_runtime_process_state_check CHECK (runtime_process_state IN ('disabled', 'ready', 'failed'));

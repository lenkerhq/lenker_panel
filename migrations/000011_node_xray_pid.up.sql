ALTER TABLE nodes ADD COLUMN xray_pid integer NOT NULL DEFAULT 0;
ALTER TABLE nodes DROP CONSTRAINT IF EXISTS nodes_runtime_process_state_check;
ALTER TABLE nodes ADD CONSTRAINT nodes_runtime_process_state_check CHECK (runtime_process_state IN ('disabled', 'ready', 'failed', 'running', 'stopped', 'restarting'));

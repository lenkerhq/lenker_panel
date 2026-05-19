#!/usr/bin/env ruby
# frozen_string_literal: true

require "yaml"

EXPECTED_OPERATIONS = {
  "/healthz" => %w[get],
  "/api/v1/auth/admin/login" => %w[post],
  "/api/v1/users" => %w[get post],
  "/api/v1/users/{id}" => %w[get patch],
  "/api/v1/users/{id}/suspend" => %w[post],
  "/api/v1/users/{id}/activate" => %w[post],
  "/api/v1/plans" => %w[get post],
  "/api/v1/plans/{id}" => %w[get patch],
  "/api/v1/plans/{id}/archive" => %w[post],
  "/api/v1/subscriptions" => %w[get post],
  "/api/v1/subscriptions/{id}" => %w[get patch],
  "/api/v1/subscriptions/{id}/access" => %w[get],
  "/api/v1/subscriptions/{id}/access-token" => %w[delete get post],
  "/api/v1/subscriptions/{id}/access-token/rotate" => %w[post],
  "/api/v1/subscriptions/{id}/handoff-invite" => %w[delete get post],
  "/api/v1/subscriptions/{id}/renew" => %w[post],
  "/api/v1/subscriptions/{id}/devices" => %w[get],
  "/api/v1/subscriptions/{id}/traffic" => %w[get],
  "/api/v1/subscriptions/{id}/quota" => %w[get post],
  "/api/v1/subscriptions/{id}/quota/reset" => %w[post],
  "/api/v1/devices/{id}" => %w[delete get],
  "/api/v1/devices/{id}/traffic" => %w[get],
  "/api/v1/devices/{id}/deactivate" => %w[post],
  "/api/v1/client/devices/register" => %w[post],
  "/api/v1/client/devices/heartbeat" => %w[post],
  "/api/v1/client/devices/me" => %w[delete],
  "/api/v1/client/handoff/claim" => %w[post],
  "/api/v1/client/subscription-access" => %w[get],
  "/api/v1/nodes" => %w[get],
  "/api/v1/nodes/bootstrap-token" => %w[post],
  "/api/v1/nodes/register" => %w[post],
  "/api/v1/nodes/{id}" => %w[get],
  "/api/v1/nodes/{id}/traffic" => %w[get],
  "/api/v1/nodes/{id}/warp" => %w[delete get post],
  "/api/v1/traffic/report" => %w[post],
  "/api/v1/warp/generate" => %w[post],
  "/api/v1/settings" => %w[get],
  "/api/v1/settings/{key}" => %w[put],
  "/api/v1/routing-rules/global" => %w[get post],
  "/api/v1/routing-rules/global/{rule_id}" => %w[delete put],
  "/api/v1/nodes/{id}/routing-rules" => %w[get post],
  "/api/v1/nodes/{id}/routing-rules/{rule_id}" => %w[delete put],
  "/api/v1/nodes/{id}/routing-rules/reorder" => %w[post],
  "/api/v1/node-profiles" => %w[get post],
  "/api/v1/node-profiles/{id}" => %w[delete get put],
  "/api/v1/node-profiles/{id}/apply/{nodeId}" => %w[post],
  "/api/v1/nodes/{id}/config-revisions" => %w[get post],
  "/api/v1/nodes/{id}/config-revisions/pending" => %w[get],
  "/api/v1/nodes/{id}/config-revisions/{revisionId}/rollback" => %w[post],
  "/api/v1/nodes/{id}/config-revisions/{revisionId}/report" => %w[post],
  "/api/v1/nodes/{id}/config-revisions/{revisionId}" => %w[get],
  "/api/v1/nodes/{id}/disable" => %w[post],
  "/api/v1/nodes/{id}/drain" => %w[post],
  "/api/v1/nodes/{id}/enable" => %w[post],
  "/api/v1/nodes/{id}/heartbeat" => %w[post],
  "/api/v1/nodes/{id}/undrain" => %w[post]
}.freeze

HTTP_METHODS = %w[get put post delete options head patch trace].freeze

def fail_validation(message)
  warn "OpenAPI validation failed: #{message}"
  exit 1
end

def expect(condition, message)
  fail_validation(message) unless condition
end

def load_yaml(path)
  YAML.load_file(path)
rescue StandardError => e
  fail_validation("cannot parse YAML: #{e.message}")
end

def fetch_ref(root, ref)
  expect(ref.start_with?("#/"), "only local refs are supported: #{ref}")

  ref.delete_prefix("#/").split("/").reduce(root) do |node, part|
    expect(node.is_a?(Hash) && node.key?(part), "unresolved ref: #{ref}")
    node[part]
  end
end

def walk_refs(root, node, path = "$")
  case node
  when Hash
    if node.key?("$ref")
      fetch_ref(root, node["$ref"])
    end
    node.each do |key, value|
      walk_refs(root, value, "#{path}.#{key}")
    end
  when Array
    node.each_with_index do |value, index|
      walk_refs(root, value, "#{path}[#{index}]")
    end
  end
end

path = ARGV.fetch(0, "docs/openapi/panel-api.v1.yaml")
spec = load_yaml(path)

expect(spec.is_a?(Hash), "root must be an object")
expect(spec["openapi"].is_a?(String), "openapi version is required")
expect(spec["openapi"].match?(/\A3\.(0|1)\.\d+\z/), "openapi version must be 3.0.x or 3.1.x")
expect(spec["info"].is_a?(Hash), "info object is required")
expect(spec.dig("info", "title").is_a?(String), "info.title is required")
expect(spec.dig("info", "version").is_a?(String), "info.version is required")
expect(spec["paths"].is_a?(Hash), "paths object is required")
expect(spec["components"].is_a?(Hash), "components object is required")
expect(spec.dig("components", "schemas").is_a?(Hash), "components.schemas object is required")
expect(spec.dig("components", "responses").is_a?(Hash), "components.responses object is required")
expect(spec.dig("components", "securitySchemes", "bearerAuth").is_a?(Hash), "bearerAuth security scheme is required")

actual_operations = {}
spec["paths"].each do |route, path_item|
  expect(path_item.is_a?(Hash), "path item for #{route} must be an object")
  methods = path_item.keys.select { |key| HTTP_METHODS.include?(key) }.sort
  actual_operations[route] = methods

  methods.each do |method|
    operation = path_item[method]
    expect(operation.is_a?(Hash), "#{method.upcase} #{route} must be an object")
    expect(operation["responses"].is_a?(Hash) && !operation["responses"].empty?, "#{method.upcase} #{route} responses are required")
  end
end

expect(actual_operations == EXPECTED_OPERATIONS, "documented operations do not match implemented panel-api surface")

expect(spec.dig("paths", "/healthz", "get", "security") == [], "GET /healthz must not require bearer auth")
expect(spec.dig("paths", "/api/v1/auth/admin/login", "post", "security") == [], "POST /api/v1/auth/admin/login must not require bearer auth")

walk_refs(spec, spec)

puts "OpenAPI validation passed: #{path}"

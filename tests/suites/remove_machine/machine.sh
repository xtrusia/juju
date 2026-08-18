wait_for_machine_removed() {
	local machine_id

	machine_id=${1}

	wait_for "false" ".machines | has(\"${machine_id}\")"
}

run_remove_machine_without_workloads() {
	echo

	file="${TEST_DIR}/remove_machine_without_workloads.log"
	ensure "remove-machine-empty" "${file}"

	juju add-machine
	wait_for_machine_agent_status "0" "started"

	juju remove-machine 0
	wait_for_machine_removed "0"

	destroy_model "remove-machine-empty"
}

run_remove_machine_with_unit() {
	echo

	file="${TEST_DIR}/remove_machine_with_unit.log"
	ensure "remove-machine-unit" "${file}"

	juju add-machine --base ubuntu@20.04
	wait_for_machine_agent_status "0" "started"
	juju deploy jameinel-ubuntu-lite --base ubuntu@20.04 --to 0
	wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite" 0 0)"

	juju remove-machine 0 --no-prompt
	wait_for_machine_removed "0"
	wait_for "0" '.applications."ubuntu-lite".units // {} | length'

	destroy_model "remove-machine-unit"
}

run_remove_machine_with_container() {
	echo

	file="${TEST_DIR}/remove_machine_with_container.log"
	ensure "remove-machine-container" "${file}"

	juju add-machine
	wait_for_machine_agent_status "0" "started"
	juju add-machine lxd:0
	wait_for_container_agent_status "0/lxd/0" "started"

	juju remove-machine 0 --no-prompt
	wait_for_machine_removed "0"

	destroy_model "remove-machine-container"
}

run_force_remove_machine_without_cloud_instance() {
	echo

	if [[ ${BOOTSTRAP_PROVIDER:-} != "lxd" ]]; then
		echo "==> TEST SKIPPED: manually removing a cloud instance is implemented for LXD only"
		return
	fi
	check_dependencies lxc

	file="${TEST_DIR}/force_remove_machine_without_cloud_instance.log"
	ensure "remove-machine-force" "${file}"

	juju add-machine
	wait_for_machine_agent_status "0" "started"
	instance_id="$(juju show-machine 0 --format=json | yq -r '.machines."0"."instance-id"')"
	lxc info "${instance_id}" >/dev/null
	lxc delete --force "${instance_id}"

	juju remove-machine 0 --force --no-wait --no-prompt
	wait_for_machine_removed "0"

	destroy_model "remove-machine-force"
}

test_remove_non_controller_machines() {
	if [ -n "$(skip 'test_remove_non_controller_machines')" ]; then
		echo "==> SKIP: Asked to skip non-controller remove-machine tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_remove_machine_without_workloads"
		run "run_remove_machine_with_unit"
		run "run_remove_machine_with_container"
		run "run_force_remove_machine_without_cloud_instance"
	)
}

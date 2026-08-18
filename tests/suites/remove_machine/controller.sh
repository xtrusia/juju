wait_for_controller_machine_count() {
	local expected attempt machine_info started stopped errors

	expected=${1}
	attempt=0

	while true; do
		machine_info="$(juju machines -m controller --format=json)"
		started="$(yq -r '[.machines[] | select(.["juju-status"].current == "started")] | length' <<<"${machine_info}")"
		stopped="$(yq -r '[.machines[] | select(.["juju-status"].current == "stopped")] | length' <<<"${machine_info}")"
		errors="$(yq -r '[.machines[] | select(.["juju-status"].current == "error")] | length' <<<"${machine_info}")"
		if [[ ${started} == "${expected}" && ${stopped} == "0" && ${errors} == "0" ]]; then
			break
		fi

		echo "[+] (attempt ${attempt}) waiting for ${expected} started controller machine(s)"
		juju machines -m controller 2>&1 | sed 's/^/    | /g' || true
		sleep "${SHORT_TIMEOUT}"
		attempt=$((attempt + 1))
		if [[ ${attempt} -gt 25 ]]; then
			echo "remove-machine failed waiting for ${expected} controller machine(s)"
			exit 1
		fi
	done
}

wait_for_controller_leader() {
	# With one controller left, leadership is the signal that the dqlite lease
	# backstop workflow has completed and the controller is usable again.
	wait_for_or_fail "timeout 5 juju exec -m controller --unit controller/leader uptime | grep -q load" 120
}

run_remove_controller_machine() {
	echo

	file="${TEST_DIR}/remove_controller_machine.log"
	ensure "remove-controller-machine" "${file}"

	juju enable-ha
	wait_for_controller_machines 3
	wait_for_ha 3

	juju switch controller
	wait_for "controller" "$(idle_condition "controller" 0 0)"
	wait_for "controller" "$(idle_condition "controller" 0 1)"
	wait_for "controller" "$(idle_condition "controller" 0 2)"

	juju remove-machine -m controller 1 --no-prompt
	wait_for_controller_machine_count 2
	juju remove-machine -m controller 2 --no-prompt
	wait_for_controller_machine_count 1

	juju show-controller --format=json |
		yq -r '[.[] | .["controller-machines"][] | select(.["instance-id"] == null)] | length' |
		check 0
	wait_for_controller_leader

	juju switch remove-controller-machine
	destroy_model "remove-controller-machine"
}

test_remove_controller_machine() {
	if [ -n "$(skip 'test_remove_controller_machine')" ]; then
		echo "==> SKIP: Asked to skip controller remove-machine tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_remove_controller_machine"
	)
}

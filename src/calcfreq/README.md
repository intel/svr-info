# Calcfreq

Calcfreq is a micro utility that can individually stress cores and figure out the actual running frequency including P0 and P1n.

Since the turbo algorithm uses the Turbo core ratios to judge what frequency the cores can run at based on how many such cores are active and at what TDP the CPU is running, its important to know if the system is adhering to this expected spec.

Many times, BIOS knobs and thermals can throw this away resulting in lower frequency and thereby performance.

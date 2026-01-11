/*
 * MPI Example for write-tracer
 *
 * This program demonstrates how to manually register MPI ranks with the
 * write-tracer REST API for monitoring.
 *
 * It uses libcurl to communicate with the REST API.
 */

#include <curl/curl.h>
#include <mpi.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>

#define TRACER_URL "http://localhost:9092"

// Helper function to register PID
void register_pid(int rank) {
  CURL *curl;
  CURLcode res;
  char url[256];
  char json_payload[64];
  pid_t pid = getpid();
  long response_code;

  snprintf(url, sizeof(url), "%s/pids", TRACER_URL);
  snprintf(json_payload, sizeof(json_payload), "{\"pid\": %d}", pid);

  curl = curl_easy_init();
  if (curl) {
    struct curl_slist *headers = NULL;
    headers = curl_slist_append(headers, "Content-Type: application/json");

    curl_easy_setopt(curl, CURLOPT_URL, url);
    curl_easy_setopt(curl, CURLOPT_HTTPHEADER, headers);
    curl_easy_setopt(curl, CURLOPT_POSTFIELDS, json_payload);

    // Suppress output
    FILE *devnull = fopen("/dev/null", "w");
    if (devnull)
      curl_easy_setopt(curl, CURLOPT_WRITEDATA, devnull);

    res = curl_easy_perform(curl);

    if (res != CURLE_OK) {
      fprintf(stderr, "[Rank %d] register_pid failed: %s\n", rank,
              curl_easy_strerror(res));
    } else {
      curl_easy_getinfo(curl, CURLINFO_RESPONSE_CODE, &response_code);
      if (response_code == 201) {
        printf("[Rank %d] Registered PID %d\n", rank, pid);
      } else {
        fprintf(stderr, "[Rank %d] Registration failed with code %ld\n", rank,
                response_code);
      }
    }

    if (devnull)
      fclose(devnull);
    curl_slist_free_all(headers);
    curl_easy_cleanup(curl);
  }
}

// Helper function to unregister PID
void unregister_pid(int rank) {
  CURL *curl;
  CURLcode res;
  char url[256];
  pid_t pid = getpid();

  snprintf(url, sizeof(url), "%s/pids/%d", TRACER_URL, pid);

  curl = curl_easy_init();
  if (curl) {
    curl_easy_setopt(curl, CURLOPT_URL, url);
    curl_easy_setopt(curl, CURLOPT_CUSTOMREQUEST, "DELETE");

    // Suppress output
    FILE *devnull = fopen("/dev/null", "w");
    if (devnull)
      curl_easy_setopt(curl, CURLOPT_WRITEDATA, devnull);

    res = curl_easy_perform(curl);
    if (res != CURLE_OK) {
      fprintf(stderr, "[Rank %d] unregister_pid failed: %s\n", rank,
              curl_easy_strerror(res));
    } else {
      printf("[Rank %d] Unregistered PID %d\n", rank, pid);
    }

    if (devnull)
      fclose(devnull);
    curl_easy_cleanup(curl);
  }
}

int main(int argc, char **argv) {
  int rank, size;
  // Default duration: 60 seconds
  int duration_sec = 60;

  MPI_Init(&argc, &argv);
  MPI_Comm_rank(MPI_COMM_WORLD, &rank);
  MPI_Comm_size(MPI_COMM_WORLD, &size);

  // Register this process
  register_pid(rank);

  // Main loop
  time_t start_time = time(NULL);
  time_t last_print = start_time;
  int iteration = 0;

  char filename[64];
  snprintf(filename, sizeof(filename), "rank_%d_output.dat", rank);

  printf("[Rank %d] Starting work loop for %d seconds...\n", rank,
         duration_sec);

  while (time(NULL) - start_time < duration_sec) {
    // Log status every 5 seconds
    if (time(NULL) - last_print >= 5) {
      printf("[Rank %d] Still running... iteration %d\n", rank, iteration);
      last_print = time(NULL);
    }

    // Perform some I/O to be traced
    FILE *fp = fopen(filename, "a");
    if (fp) {
      fprintf(fp, "Iteration %d data\n", iteration);
      fclose(fp);
    }

    iteration++;
    usleep(100000); // Sleep 100ms
  }

  // Unregister
  unregister_pid(rank);

  MPI_Finalize();
  return 0;
}

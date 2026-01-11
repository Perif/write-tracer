/*
 * NCCL Example for write-tracer
 *
 * This program demonstrates how to manually register NCCL ranks with the
 * write-tracer REST API for monitoring.
 *
 * It uses libcurl to communicate with the REST API.
 */

#include <cuda_runtime.h>
#include <curl/curl.h>
#include <mpi.h>
#include <nccl.h>
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

  // Initialize MPI
  MPI_Init(&argc, &argv);
  MPI_Comm_rank(MPI_COMM_WORLD, &rank);
  MPI_Comm_size(MPI_COMM_WORLD, &size);

  // Register this process
  register_pid(rank);

  // Initialize NCCL
  ncclComm_t comm;
  ncclUniqueId id;
  if (rank == 0)
    ncclGetUniqueId(&id);
  MPI_Bcast(&id, sizeof(id), MPI_BYTE, 0, MPI_COMM_WORLD);

  // Pick CUDA device
  int cudaDev = rank % 8; // Assuming max 8 GPUs per node for this example
  cudaSetDevice(cudaDev);

  ncclCommInitRank(&comm, size, id, rank);

  // Data buffers
  float *sendbuff, *recvbuff;
  size_t data_size = 32 * 1024 * 1024; // 32 MB
  cudaMalloc(&sendbuff, data_size * sizeof(float));
  cudaMalloc(&recvbuff, data_size * sizeof(float));
  cudaStream_t stream;
  cudaStreamCreate(&stream);

  // Main loop
  time_t start_time = time(NULL);
  time_t last_print = start_time;
  int iteration = 0;

  printf("[Rank %d] Starting NCCL loop for %d seconds...\n", rank,
         duration_sec);

  while (time(NULL) - start_time < duration_sec) {
    // Log status every 5 seconds
    if (time(NULL) - last_print >= 5) {
      printf("[Rank %d] Still running... iteration %d\n", rank, iteration);
      last_print = time(NULL);
    }

    // Perform NCCL AllReduce
    ncclAllReduce(sendbuff, recvbuff, data_size, ncclFloat, ncclSum, comm,
                  stream);
    cudaStreamSynchronize(stream);

    iteration++;
    // Small sleep to not hammer the GPU too hard in this toy example
    usleep(10000);
  }

  // Unregister
  unregister_pid(rank);

  // Cleanup
  cudaFree(sendbuff);
  cudaFree(recvbuff);
  ncclCommDestroy(comm);
  MPI_Finalize();

  return 0;
}

cmake_minimum_required(VERSION 3.16)

# Project-local components (board, k210_link, k210_port, ...)
set(EXTRA_COMPONENT_DIRS "${CMAKE_CURRENT_LIST_DIR}/components")

include($ENV{IDF_PATH}/tools/cmake/project.cmake)
project(emo_open_fw)

# Pinned toolchain: ESP-IDF 5.2.x (cJSON stays in-tree as components/json; APIs checked against release/v5.2).
if(DEFINED IDF_VERSION_MAJOR AND NOT (IDF_VERSION_MAJOR EQUAL 5 AND IDF_VERSION_MINOR EQUAL 2))
    message(FATAL_ERROR "emo-open-fw is pinned to ESP-IDF 5.2.x, found ${IDF_VERSION_MAJOR}.${IDF_VERSION_MINOR}")
endif()

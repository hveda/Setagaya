// Setagaya Alpine.js App - Phase 2
// Vue.js to Alpine.js conversion with routing and RBAC integration

// Simple router for Alpine.js
class AlpineRouter {
  constructor() {
    this.currentRoute = '/';
    this.currentParams = {};
    this.routes = {
      '/': () => this.renderProjects(),
      '/projects': () => this.renderProjects(),
      '/collections/:id': (params) => this.renderCollection(params.id),
      '/plans/:id': (params) => this.renderPlan(params.id),
      '/monitoring': () => this.renderMonitoring(),
      '/admin': () => this.renderAdmin(),
      '/admin/collections': () => this.renderAdminCollections(),
      '/admin/users': () => this.renderAdminUsers(),
      '/admin/roles': () => this.renderAdminRoles()
    };
    
    // Listen for hash changes
    window.addEventListener('hashchange', () => this.handleRoute());
    window.addEventListener('load', () => this.handleRoute());
  }

  handleRoute() {
    const hash = window.location.hash.slice(1) || '/';
    this.navigateTo(hash);
  }

  navigateTo(path) {
    this.currentRoute = path;
    this.currentParams = {};
    
    // Update app's current route if available
    if (this.app && this.app.setCurrentRoute) {
      this.app.setCurrentRoute(path);
    }
    
    // Simple parameter matching
    for (const [route, handler] of Object.entries(this.routes)) {
      const routePattern = route.replace(/:([^/]+)/g, '([^/]+)');
      const regex = new RegExp(`^${routePattern}$`);
      const match = path.match(regex);
      
      if (match) {
        // Extract parameters
        const paramNames = (route.match(/:([^/]+)/g) || []).map(p => p.slice(1));
        const paramValues = match.slice(1);
        
        paramNames.forEach((name, index) => {
          this.currentParams[name] = paramValues[index];
        });
        
        handler(this.currentParams);
        return;
      }
    }
    
    // Default to projects page
    this.renderProjects();
  }

  renderProjects() {
    document.querySelector('#dynamic-content').innerHTML = `
      <div x-data="projectsComponent()" x-init="init()" @project-deleted="onProjectDeleted" @collection-created="onCollectionCreated" @plan-created="onPlanCreated">
        <div x-show="loading" class="text-center py-8">
          <div class="text-gray-500">Loading projects...</div>
        </div>
        
        <div x-show="!loading">
          <!-- Breadcrumb -->
          <nav aria-label="breadcrumb" class="mb-4">
            <ol class="breadcrumb list-none p-0 inline-flex">
              <li class="breadcrumb-item text-blue-600">Projects</li>
            </ol>
          </nav>

          <!-- Projects List -->
          <div class="project-list">
            <template x-for="project in projects" :key="project.id">
              <div x-data="projectComponent(project)" class="card mt-3 bg-white shadow rounded-lg">
                <div class="card-body p-6">
                  <h5 class="card-title text-xl font-semibold mb-2" x-text="project.name"></h5>
                  <div class="card-subtitle mb-4 text-gray-500 text-sm">
                    <span>Project ID: </span><span x-text="project.id"></span>
                    <br>
                    <span>Owner: </span><span x-text="project.owner"></span>
                    <button x-show-if-permission="'projects:delete'" 
                            @click="deleteProject()" 
                            class="float-right btn btn-outline-danger text-red-600 hover:text-red-800 border border-red-300 px-3 py-1 rounded text-sm">
                      Delete
                    </button>
                  </div>
                  
                  <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
                    <!-- Collections -->
                    <div class="card bg-gray-50 rounded-lg">
                      <div class="card-body p-4">
                        <h5 class="card-title text-lg font-medium mb-3">Collections</h5>
                        <ul class="list-none">
                          <template x-for="c in project.collections" :key="c.id">
                            <li class="inline-block mr-4 mb-2">
                              <a :href="'#/collections/' + c.id" 
                                 class="text-blue-600 hover:text-blue-800" 
                                 x-text="c.name"></a>
                            </li>
                          </template>
                        </ul>
                        <button x-show-if-permission="'collections:create'" 
                                @click="showNewCollectionModal()" 
                                class="btn btn-outline-default border border-gray-300 px-3 py-2 rounded text-sm hover:bg-gray-50">
                          New Collection
                        </button>
                        
                        <!-- New Collection Modal -->
                        <div x-show="creating_collection" 
                             x-transition:enter="ease-out duration-300"
                             class="fixed inset-0 bg-gray-600 bg-opacity-50 overflow-y-auto h-full w-full z-50">
                          <div class="relative top-20 mx-auto p-5 border w-96 shadow-lg rounded-md bg-white">
                            <h3 class="text-lg font-bold text-gray-900 mb-4">New Collection</h3>
                            <form @submit.prevent="createCollection()">
                              <div class="mb-4">
                                <label class="block text-sm font-medium text-gray-700 mb-2">Collection Name</label>
                                <input x-model="newCollectionForm.name" 
                                       type="text" 
                                       required
                                       class="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500">
                              </div>
                              <div class="flex justify-end space-x-2">
                                <button type="button" 
                                        @click="creating_collection = false"
                                        class="px-4 py-2 text-gray-600 border border-gray-300 rounded-md hover:bg-gray-50">
                                  Cancel
                                </button>
                                <button type="submit"
                                        class="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700">
                                  Create
                                </button>
                              </div>
                            </form>
                          </div>
                        </div>
                      </div>
                    </div>
                    
                    <!-- Plans -->
                    <div class="card bg-gray-50 rounded-lg">
                      <div class="card-body p-4">
                        <h5 class="card-title text-lg font-medium mb-3">Plans</h5>
                        <ul class="list-none">
                          <template x-for="p in project.plans" :key="p.id">
                            <li class="inline-block mr-4 mb-2">
                              <a :href="'#/plans/' + p.id" 
                                 class="text-blue-600 hover:text-blue-800" 
                                 x-text="p.name"></a>
                            </li>
                          </template>
                        </ul>
                        <button x-show-if-permission="'plans:create'" 
                                @click="showNewPlanModal()" 
                                class="btn btn-outline-default border border-gray-300 px-3 py-2 rounded text-sm hover:bg-gray-50">
                          New Plan
                        </button>
                        
                        <!-- New Plan Modal -->
                        <div x-show="creating_plan" 
                             x-transition:enter="ease-out duration-300"
                             class="fixed inset-0 bg-gray-600 bg-opacity-50 overflow-y-auto h-full w-full z-50">
                          <div class="relative top-20 mx-auto p-5 border w-96 shadow-lg rounded-md bg-white">
                            <h3 class="text-lg font-bold text-gray-900 mb-4">New Plan</h3>
                            <form @submit.prevent="createPlan()">
                              <div class="mb-4">
                                <label class="block text-sm font-medium text-gray-700 mb-2">Plan Name</label>
                                <input x-model="newPlanForm.name" 
                                       type="text" 
                                       required
                                       class="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500">
                              </div>
                              <div class="flex justify-end space-x-2">
                                <button type="button" 
                                        @click="creating_plan = false"
                                        class="px-4 py-2 text-gray-600 border border-gray-300 rounded-md hover:bg-gray-50">
                                  Cancel
                                </button>
                                <button type="submit"
                                        class="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700">
                                  Create
                                </button>
                              </div>
                            </form>
                          </div>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </template>
            
            <!-- Create Project Button -->
            <button x-show-if-permission="'projects:create'" 
                    @click="showCreateModal()" 
                    class="btn btn-outline-primary mt-3 border border-blue-600 text-blue-600 px-4 py-2 rounded hover:bg-blue-50">
              Create Project
            </button>
            
            <!-- Create Project Modal -->
            <div x-show="creating" 
                 x-transition:enter="ease-out duration-300"
                 class="fixed inset-0 bg-gray-600 bg-opacity-50 overflow-y-auto h-full w-full z-50">
              <div class="relative top-20 mx-auto p-5 border w-96 shadow-lg rounded-md bg-white">
                <h3 class="text-lg font-bold text-gray-900 mb-4">Create New Project</h3>
                <form @submit.prevent="createProject()">
                  <div class="mb-4">
                    <label class="block text-sm font-medium text-gray-700 mb-2">Project Name</label>
                    <input x-model="newProjectForm.name" 
                           type="text" 
                           required
                           class="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500">
                  </div>
                  <div class="mb-4">
                    <label class="block text-sm font-medium text-gray-700 mb-2">Project Owner</label>
                    <input x-model="newProjectForm.owner" 
                           type="text" 
                           required
                           class="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500">
                  </div>
                  <div class="flex justify-end space-x-2">
                    <button type="button" 
                            @click="creating = false"
                            class="px-4 py-2 text-gray-600 border border-gray-300 rounded-md hover:bg-gray-50">
                      Cancel
                    </button>
                    <button type="submit"
                            class="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700">
                      Create
                    </button>
                  </div>
                </form>
              </div>
            </div>
          </div>
        </div>
      </div>
    `;
  }

  renderCollection(collectionId) {
    document.querySelector('#dynamic-content').innerHTML = `
      <div x-data="collectionComponent('${collectionId}')" x-init="init()">
        <div x-show="loading" class="text-center py-8">
          <div class="text-gray-500">Loading collection...</div>
        </div>
        
        <div x-show="!loading">
          <!-- Breadcrumb -->
          <nav aria-label="breadcrumb" class="mb-4">
            <ol class="breadcrumb list-none p-0 inline-flex">
              <li class="breadcrumb-item"><a href="#/" class="text-blue-600 hover:text-blue-800">Projects</a></li>
              <li class="breadcrumb-item mx-2">/</li>
              <li class="breadcrumb-item">Collections</li>
              <li class="breadcrumb-item mx-2">/</li>
              <li class="breadcrumb-item" x-text="collection.name"></li>
            </ol>
          </nav>

          <!-- Collection Details -->
          <div class="card bg-white shadow rounded-lg">
            <div class="card-header bg-gray-50 p-4 border-b">
              <h3 class="text-xl font-semibold mb-2" x-text="collection.name"></h3>
              <h6 class="text-sm text-gray-500 mb-4">Collection ID: <span x-text="collection.id"></span></h6>
              
              <!-- Action Buttons -->
              <div class="space-x-2">
                <button x-show-if-permission="'collections:execute'" 
                        @click="launch()" 
                        :disabled="!launchable"
                        class="btn btn-outline-primary border border-blue-600 text-blue-600 px-3 py-2 rounded hover:bg-blue-50 disabled:opacity-50">
                  Launch
                </button>
                <button x-show-if-permission="'collections:execute'" 
                        @click="trigger()" 
                        :disabled="!triggerable"
                        class="btn btn-outline-primary border border-blue-600 text-blue-600 px-3 py-2 rounded hover:bg-blue-50 disabled:opacity-50">
                  Trigger
                </button>
                <button x-show-if-permission="'collections:execute'" 
                        @click="stop()" 
                        :disabled="!stoppable"
                        class="btn btn-outline-primary border border-blue-600 text-blue-600 px-3 py-2 rounded hover:bg-blue-50 disabled:opacity-50">
                  Stop
                </button>
                <button x-show-if-permission="'collections:execute'" 
                        @click="purge()" 
                        class="btn btn-outline-primary border border-blue-600 text-blue-600 px-3 py-2 rounded hover:bg-blue-50">
                  Purge
                </button>
                <button x-show-if-permission="'collections:delete'" 
                        @click="deleteCollection()" 
                        class="btn btn-outline-danger border border-red-600 text-red-600 px-3 py-2 rounded hover:bg-red-50 float-right">
                  Delete
                </button>
              </div>
              
              <!-- Status Badges -->
              <div class="mt-2">
                <span x-show="trigger_in_progress" class="badge bg-yellow-100 text-yellow-800 px-2 py-1 rounded-full text-xs">Tests are being started</span>
                <span x-show="stop_in_progress" class="badge bg-yellow-100 text-yellow-800 px-2 py-1 rounded-full text-xs">Tests are being stopped</span>
                <span x-show="purge_tip" class="badge bg-yellow-100 text-yellow-800 px-2 py-1 rounded-full text-xs">Engines are being purged</span>
              </div>
            </div>
            
            <!-- Collection body with plans table would go here -->
            <div class="card-body p-6">
              <p class="text-gray-600">Collection details and execution plans will be displayed here.</p>
              <p class="text-sm text-gray-500 mt-2">Full collection interface with file uploads, execution plans table, and monitoring will be implemented in the next iteration.</p>
            </div>
          </div>
        </div>
      </div>
    `;
  }

  renderPlan(planId) {
    document.querySelector('#dynamic-content').innerHTML = `
      <div x-data="planComponent('${planId}')" x-init="init()">
        <div x-show="loading" class="text-center py-8">
          <div class="text-gray-500">Loading plan...</div>
        </div>
        
        <div x-show="!loading">
          <!-- Breadcrumb -->
          <nav aria-label="breadcrumb" class="mb-4">
            <ol class="breadcrumb list-none p-0 inline-flex">
              <li class="breadcrumb-item"><a href="#/" class="text-blue-600 hover:text-blue-800">Projects</a></li>
              <li class="breadcrumb-item mx-2">/</li>
              <li class="breadcrumb-item">Plan</li>
              <li class="breadcrumb-item mx-2">/</li>
              <li class="breadcrumb-item" x-text="plan.name"></li>
            </ol>
          </nav>

          <!-- Plan Details -->
          <div class="card bg-white shadow rounded-lg">
            <div class="card-body p-6">
              <button x-show-if-permission="'plans:delete'" 
                      @click="deletePlan()" 
                      class="btn btn-outline-danger border border-red-600 text-red-600 px-3 py-2 rounded hover:bg-red-50 float-right">
                Delete
              </button>
              
              <h5 class="card-title text-xl font-semibold mb-2" x-text="plan.name"></h5>
              <h6 class="card-subtitle mb-4 text-gray-500 text-sm">Plan ID: <span x-text="plan.id"></span></h6>
              
              <!-- File Upload Section -->
              <div class="mb-4">
                <label x-show-if-permission="'plans:update'" 
                       for="planFile" 
                       class="btn btn-outline-dark border border-gray-600 text-gray-600 px-3 py-2 rounded hover:bg-gray-50 cursor-pointer">
                  📁 Upload File
                </label>
                <input x-show-if-permission="'plans:update'" 
                       type="file" 
                       id="planFile" 
                       @change="handleFileUpload($event)" 
                       accept=".csv, .jmx, .txt, .json" 
                       class="hidden">
              </div>
              
              <div class="alert bg-blue-50 border border-blue-200 text-blue-800 p-3 rounded mb-4">
                <p class="mb-0">You can upload only one .jmx file per plan</p>
              </div>
              
              <!-- Test File Display -->
              <template x-if="plan.test_file">
                <div class="mb-4">
                  <div class="btn-group inline-flex">
                    <a :href="getFileDownloadUrl(plan.test_file)" 
                       target="_blank" 
                       class="btn btn-outline-success border border-green-600 text-green-600 px-3 py-2 rounded-l hover:bg-green-50" 
                       x-text="plan.test_file.filename"></a>
                    <button x-show-if-permission="'plans:update'" 
                            @click="deletePlanFile(plan.test_file.filename)" 
                            class="btn btn-outline-success border border-green-600 text-green-600 px-2 py-2 rounded-r hover:bg-green-50">
                      ✕
                    </button>
                  </div>
                </div>
              </template>
              
              <!-- Data Files Display -->
              <template x-if="plan.data && plan.data.length > 0">
                <div class="mb-4">
                  <template x-for="data in plan.data" :key="data.filename">
                    <div class="btn-group inline-flex mr-2 mb-2">
                      <a :href="getFileDownloadUrl(data)" 
                         target="_blank" 
                         class="btn btn-outline-dark border border-gray-600 text-gray-600 px-3 py-2 rounded-l hover:bg-gray-50" 
                         x-text="data.filename"></a>
                      <button x-show-if-permission="'plans:update'" 
                              @click="deletePlanFile(data.filename)" 
                              class="btn btn-outline-dark border border-gray-600 text-gray-600 px-2 py-2 rounded-r hover:bg-gray-50">
                        ✕
                      </button>
                    </div>
                  </template>
                </div>
              </template>
            </div>
          </div>
        </div>
      </div>
    `;
  }

  renderMonitoring() {
    document.querySelector('#dynamic-content').innerHTML = `
      <div class="space-y-6">
        <!-- Breadcrumb -->
        <nav aria-label="breadcrumb" class="mb-4">
          <ol class="breadcrumb list-none p-0 inline-flex">
            <li class="breadcrumb-item text-blue-600">Monitoring</li>
          </ol>
        </nav>

        <!-- Monitoring Dashboard -->
        <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
          <!-- Results Dashboard -->
          <div class="card bg-white shadow rounded-lg">
            <div class="card-body p-6">
              <h3 class="text-lg font-semibold mb-4">
                <i class="fas fa-chart-bar mr-2 text-blue-600"></i>
                Results Dashboard
              </h3>
              <p class="text-gray-600 mb-4">View test execution results and performance metrics</p>
              <a href="${window.result_dashboard}" 
                 target="_blank" 
                 class="btn-setagaya inline-flex items-center">
                <i class="fas fa-external-link-alt mr-2"></i>
                Open Dashboard
              </a>
            </div>
          </div>

          <!-- Engine Health -->
          <div class="card bg-white shadow rounded-lg">
            <div class="card-body p-6">
              <h3 class="text-lg font-semibold mb-4">
                <i class="fas fa-heartbeat mr-2 text-green-600"></i>
                Engine Health
              </h3>
              <p class="text-gray-600 mb-4">Monitor JMeter engine status and performance</p>
              ${window.engine_health_dashboard ? `
                <a href="${window.engine_health_dashboard}" 
                   target="_blank" 
                   class="btn-setagaya inline-flex items-center">
                  <i class="fas fa-external-link-alt mr-2"></i>
                  Open Health Dashboard
                </a>
              ` : `
                <p class="text-sm text-gray-500">Health dashboard not configured</p>
              `}
            </div>
          </div>
        </div>

        <!-- System Information -->
        <div class="card bg-white shadow rounded-lg">
          <div class="card-body p-6">
            <h3 class="text-lg font-semibold mb-4">
              <i class="fas fa-info-circle mr-2 text-gray-600"></i>
              System Information
            </h3>
            <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
              <div class="bg-gray-50 rounded-lg p-4">
                <div class="text-sm text-gray-500">Environment</div>
                <div class="text-lg font-medium">${window.running_context}</div>
              </div>
              <div class="bg-gray-50 rounded-lg p-4">
                <div class="text-sm text-gray-500">GC Interval</div>
                <div class="text-lg font-medium">${window.gcDuration}s</div>
              </div>
              <div class="bg-gray-50 rounded-lg p-4">
                <div class="text-sm text-gray-500">Session ID</div>
                <div class="text-lg font-medium">${window.enable_sid ? 'Enabled' : 'Disabled'}</div>
              </div>
            </div>
          </div>
        </div>
      </div>
    `;
  }

  renderAdmin() {
    document.querySelector('#admin-content').innerHTML = `
      <div x-data="adminRootComponent()" x-init="init()" class="space-y-6">
        <!-- Admin Breadcrumb -->
        <nav aria-label="breadcrumb" class="mb-4">
          <ol class="breadcrumb list-none p-0 inline-flex">
            <li class="breadcrumb-item text-blue-600">Admin Dashboard</li>
          </ol>
        </nav>

        <!-- Admin Overview -->
        <div class="bg-gradient-to-r from-blue-50 to-indigo-50 rounded-lg p-6 border border-blue-200">
          <div class="flex items-center justify-between">
            <div>
              <h2 class="text-2xl font-bold text-gray-900">
                <i class="fas fa-shield-alt mr-2 text-blue-600"></i>
                Administration Panel
              </h2>
              <p class="mt-2 text-gray-600">Manage users, roles, permissions, and system configuration</p>
            </div>
            <div class="text-right">
              <div class="text-sm text-gray-500">Current User: <span class="font-medium">${window.user_account}</span></div>
              <div class="text-sm text-gray-500 mt-1">Admin Access: Granted</div>
            </div>
          </div>
        </div>

        <!-- Admin Navigation -->
        <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
          <!-- User Management -->
          <div class="card bg-white shadow rounded-lg hover:shadow-lg transition-shadow cursor-pointer" 
               @click="window.location.hash = '#/admin/users'">
            <div class="card-body p-6 text-center">
              <div class="text-4xl text-blue-600 mb-4">
                <i class="fas fa-users"></i>
              </div>
              <h3 class="text-lg font-semibold mb-2">User Management</h3>
              <p class="text-gray-600 text-sm mb-4">Manage user accounts and assignments</p>
              <div class="bg-blue-50 rounded-lg p-3">
                <div class="text-2xl font-bold text-blue-700" x-text="userStats.totalUsers">-</div>
                <div class="text-sm text-blue-600">Total Users</div>
              </div>
            </div>
          </div>

          <!-- Role Management -->
          <div class="card bg-white shadow rounded-lg hover:shadow-lg transition-shadow cursor-pointer"
               @click="window.location.hash = '#/admin/roles'">
            <div class="card-body p-6 text-center">
              <div class="text-4xl text-green-600 mb-4">
                <i class="fas fa-user-tag"></i>
              </div>
              <h3 class="text-lg font-semibold mb-2">Role Management</h3>
              <p class="text-gray-600 text-sm mb-4">Configure roles and permissions</p>
              <div class="bg-green-50 rounded-lg p-3">
                <div class="text-2xl font-bold text-green-700">4</div>
                <div class="text-sm text-green-600">System Roles</div>
              </div>
            </div>
          </div>

          <!-- System Collections -->
          <div class="card bg-white shadow rounded-lg hover:shadow-lg transition-shadow cursor-pointer"
               @click="window.location.hash = '#/admin/collections'">
            <div class="card-body p-6 text-center">
              <div class="text-4xl text-yellow-600 mb-4">
                <i class="fas fa-layer-group"></i>
              </div>
              <h3 class="text-lg font-semibold mb-2">Running Collections</h3>
              <p class="text-gray-600 text-sm mb-4">Monitor active test collections</p>
              <div class="bg-yellow-50 rounded-lg p-3">
                <div class="text-2xl font-bold text-yellow-700" x-text="systemStats.runningCollections">-</div>
                <div class="text-sm text-yellow-600">Active Collections</div>
              </div>
            </div>
          </div>
        </div>

        <!-- Quick Actions -->
        <div class="card bg-white shadow rounded-lg">
          <div class="card-body p-6">
            <h3 class="text-lg font-semibold mb-4">
              <i class="fas fa-bolt mr-2 text-gray-600"></i>
              Quick Actions
            </h3>
            <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
              <button @click="refreshStats()" 
                      class="btn-outline-setagaya text-left p-4 rounded-lg border-2 border-dashed">
                <i class="fas fa-sync-alt mr-2"></i>
                Refresh System Statistics
              </button>
              <button @click="exportConfig()" 
                      class="btn-outline-setagaya text-left p-4 rounded-lg border-2 border-dashed">
                <i class="fas fa-download mr-2"></i>
                Export System Configuration
              </button>
            </div>
          </div>
        </div>
      </div>
    `;
  }

  renderAdminCollections() {
    // Placeholder for admin collections page
    document.querySelector('#admin-content').innerHTML = `
      <div class="space-y-6">
        <nav aria-label="breadcrumb" class="mb-4">
          <ol class="breadcrumb list-none p-0 inline-flex">
            <li class="breadcrumb-item"><a href="#/admin" class="text-blue-600 hover:text-blue-800">Admin</a></li>
            <li class="breadcrumb-item mx-2">/</li>
            <li class="breadcrumb-item">Collections</li>
          </ol>
        </nav>
        
        <div class="card bg-white shadow rounded-lg">
          <div class="card-body p-6">
            <div class="flex justify-between items-center mb-4">
              <h3 class="text-xl font-semibold">Running Collections</h3>
              <button class="btn-setagaya">
                <i class="fas fa-sync-alt mr-2"></i>Refresh
              </button>
            </div>
            <p class="text-gray-600">Admin collections interface will be implemented here.</p>
            <div class="mt-4 p-4 bg-blue-50 rounded-lg">
              <p class="text-sm text-blue-800">
                <i class="fas fa-info-circle mr-2"></i>
                This will show all running test collections across the system with the ability to monitor and control them.
              </p>
            </div>
          </div>
        </div>
      </div>
    `;
  }

  renderAdminUsers() {
    document.querySelector('#admin-content').innerHTML = `
      <div x-data="userManagementComponent()" x-init="init()" class="space-y-6">
        <nav aria-label="breadcrumb" class="mb-4">
          <ol class="breadcrumb list-none p-0 inline-flex">
            <li class="breadcrumb-item"><a href="#/admin" class="text-blue-600 hover:text-blue-800">Admin</a></li>
            <li class="breadcrumb-item mx-2">/</li>
            <li class="breadcrumb-item">Users</li>
          </ol>
        </nav>
        
        <div class="card bg-white shadow rounded-lg">
          <div class="card-body p-6">
            <div class="flex justify-between items-center mb-6">
              <h3 class="text-xl font-semibold">User Management</h3>
              <button x-show-if-permission="'users:manage'" 
                      @click="showCreateUserModal()" 
                      class="btn-setagaya">
                <i class="fas fa-plus mr-2"></i>Add User
              </button>
            </div>
            
            <!-- Users Table -->
            <div class="overflow-x-auto">
              <table class="w-full text-sm text-left">
                <thead class="text-xs text-gray-700 uppercase bg-gray-50">
                  <tr>
                    <th scope="col" class="px-6 py-3">User</th>
                    <th scope="col" class="px-6 py-3">Roles</th>
                    <th scope="col" class="px-6 py-3">Status</th>
                    <th scope="col" class="px-6 py-3">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  <template x-for="user in users" :key="user.id">
                    <tr class="bg-white border-b hover:bg-gray-50">
                      <td class="px-6 py-4 font-medium text-gray-900">
                        <div>
                          <div x-text="user.name"></div>
                          <div class="text-sm text-gray-500" x-text="user.username"></div>
                        </div>
                      </td>
                      <td class="px-6 py-4">
                        <template x-for="role in user.roles" :key="role.id">
                          <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-800 mr-1"
                                x-text="role.name"></span>
                        </template>
                      </td>
                      <td class="px-6 py-4">
                        <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium"
                              :class="user.active ? 'bg-green-100 text-green-800' : 'bg-red-100 text-red-800'"
                              x-text="user.active ? 'Active' : 'Inactive'"></span>
                      </td>
                      <td class="px-6 py-4">
                        <div class="flex space-x-2">
                          <button x-show-if-permission="'users:manage'" 
                                  @click="editUser(user)" 
                                  class="text-blue-600 hover:text-blue-800">
                            <i class="fas fa-edit"></i>
                          </button>
                          <button x-show-if-permission="'users:manage'" 
                                  @click="deleteUser(user)" 
                                  class="text-red-600 hover:text-red-800">
                            <i class="fas fa-trash"></i>
                          </button>
                        </div>
                      </td>
                    </tr>
                  </template>
                </tbody>
              </table>
            </div>
            
            <div x-show="users.length === 0" class="text-center py-8">
              <p class="text-gray-500">No users found. This feature requires RBAC API implementation.</p>
            </div>
          </div>
        </div>
      </div>
    `;
  }

  renderAdminRoles() {
    document.querySelector('#admin-content').innerHTML = `
      <div x-data="roleManagementComponent()" x-init="init()" class="space-y-6">
        <nav aria-label="breadcrumb" class="mb-4">
          <ol class="breadcrumb list-none p-0 inline-flex">
            <li class="breadcrumb-item"><a href="#/admin" class="text-blue-600 hover:text-blue-800">Admin</a></li>
            <li class="breadcrumb-item mx-2">/</li>
            <li class="breadcrumb-item">Roles</li>
          </ol>
        </nav>
        
        <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <!-- Roles List -->
          <div class="card bg-white shadow rounded-lg">
            <div class="card-body p-6">
              <div class="flex justify-between items-center mb-6">
                <h3 class="text-lg font-semibold">System Roles</h3>
                <button x-show-if-permission="'roles:manage'" 
                        @click="showCreateRoleModal()" 
                        class="btn-setagaya">
                  <i class="fas fa-plus mr-2"></i>Add Role
                </button>
              </div>
              
              <div class="space-y-3">
                <template x-for="role in roles" :key="role.id">
                  <div class="border rounded-lg p-4 hover:bg-gray-50 cursor-pointer"
                       @click="selectRole(role)"
                       :class="{ 'border-blue-500 bg-blue-50': selectedRole && selectedRole.id === role.id }">
                    <div class="flex justify-between items-center">
                      <div>
                        <h4 class="font-medium" x-text="role.name"></h4>
                        <p class="text-sm text-gray-500" x-text="role.description"></p>
                      </div>
                      <div class="text-sm text-gray-400">
                        <span x-text="role.permissions ? role.permissions.length : 0"></span> permissions
                      </div>
                    </div>
                  </div>
                </template>
              </div>
              
              <div x-show="roles.length === 0" class="text-center py-8">
                <p class="text-gray-500">No roles found. This feature requires RBAC API implementation.</p>
              </div>
            </div>
          </div>
          
          <!-- Role Details/Permissions -->
          <div class="card bg-white shadow rounded-lg">
            <div class="card-body p-6">
              <h3 class="text-lg font-semibold mb-6">Role Permissions</h3>
              
              <div x-show="!selectedRole" class="text-center py-8">
                <p class="text-gray-500">Select a role to view and edit permissions</p>
              </div>
              
              <div x-show="selectedRole">
                <div class="mb-4">
                  <h4 class="font-medium mb-2" x-text="selectedRole?.name"></h4>
                  <p class="text-sm text-gray-600" x-text="selectedRole?.description"></p>
                </div>
                
                <div class="space-y-4">
                  <h5 class="font-medium">Permissions</h5>
                  <div class="max-h-64 overflow-y-auto">
                    <template x-for="permission in availablePermissions" :key="permission.name">
                      <label class="flex items-center p-2 hover:bg-gray-50 rounded">
                        <input type="checkbox" 
                               :checked="roleHasPermission(permission.name)"
                               @change="togglePermission(permission.name)"
                               class="mr-3">
                        <div>
                          <div class="font-medium text-sm" x-text="permission.name"></div>
                          <div class="text-xs text-gray-500" x-text="permission.description"></div>
                        </div>
                      </label>
                    </template>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    `;
  }
}

// Main Alpine.js app data and methods
function shibuyaApp() {
  return {
    // Application state
    user: null,
    initialized: false,
    router: null,
    currentRoute: '/',
    
    // Initialize the app
    async init() {
      try {
        // Initialize auth manager
        await window.authManager.init();
        this.user = window.authManager.user;
        
        // Initialize router
        this.router = new AlpineRouter();
        this.router.app = this; // Reference for updating currentRoute
        
        // Set initial route
        this.currentRoute = window.location.hash.slice(1) || '/';
        
        this.initialized = true;
        console.log('Setagaya app initialized with user:', this.user);
      } catch (error) {
        console.error('Failed to initialize app:', error);
        this.initialized = true; // Still mark as initialized to show UI
      }
    },
    
    // Update current route (called by router)
    setCurrentRoute(route) {
      this.currentRoute = route;
    },
    
    // Auth methods
    async logout() {
      if (window.authManager) {
        window.authManager.logout();
      }
    },
    
    // Permission helpers for templates
    can(permission) {
      return window.authManager && window.authManager.hasPermission(permission);
    },
    
    hasRole(role) {
      return window.authManager && window.authManager.hasRole(role);
    },
    
    isAdmin() {
      return window.authManager && window.authManager.isAdmin();
    },
    
    // Resource permission helpers
    canCreateProject() {
      return this.can('projects:create');
    },
    
    canManageProjects() {
      return this.can('projects:update') || this.can('projects:delete');
    },
    
    canCreateCollection() {
      return this.can('collections:create');
    },
    
    canManageCollections() {
      return this.can('collections:update') || this.can('collections:delete');
    },
    
    canCreatePlan() {
      return this.can('plans:create');
    },
    
    canManagePlans() {
      return this.can('plans:update') || this.can('plans:delete');
    },
    
    canManageUsers() {
      return this.can('users:manage') || this.isAdmin();
    },
    
    canManageRoles() {
      return this.can('roles:manage') || this.isAdmin();
    }
  };
}

// Make app function globally available
window.shibuyaApp = shibuyaApp;

// Initialize Alpine.js when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
  console.log('Setagaya Alpine.js app loading...');
});

// For backward compatibility during transition
// Keep some constants that might be referenced
if (typeof SYNC_INTERVAL === 'undefined') {
  window.SYNC_INTERVAL = 5000; // 5 seconds
}

if (typeof enable_sid === 'undefined') {
  window.enable_sid = false;
}
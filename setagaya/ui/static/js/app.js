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
      '/admin': () => this.renderAdmin(),
      '/admin/collections': () => this.renderAdminCollections()
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
    document.querySelector('.shibuya').innerHTML = `
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
    document.querySelector('.shibuya').innerHTML = `
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
    document.querySelector('.shibuya').innerHTML = `
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

  renderAdmin() {
    document.querySelector('.shibuya-admin').innerHTML = `
      <div class="card bg-white shadow rounded-lg">
        <div class="card-body p-6">
          <div class="card-title text-xl font-semibold mb-4">Admin pages</div>
          <ol class="list-group space-y-2">
            <li class="list-group-item">
              <a href="#/admin/collections" class="text-blue-600 hover:text-blue-800">Collections</a>
            </li>
          </ol>
        </div>
      </div>
    `;
  }

  renderAdminCollections() {
    // Placeholder for admin collections page
    document.querySelector('.shibuya-admin').innerHTML = `
      <div>
        <nav aria-label="breadcrumb" class="mb-4">
          <ol class="breadcrumb list-none p-0 inline-flex">
            <li class="breadcrumb-item"><a href="#/admin" class="text-blue-600 hover:text-blue-800">Admin</a></li>
            <li class="breadcrumb-item mx-2">/</li>
            <li class="breadcrumb-item">collections</li>
          </ol>
        </nav>
        <div class="card bg-white shadow rounded-lg">
          <div class="card-body p-6">
            <div class="card-title text-xl font-semibold mb-4">Running Collections</div>
            <p class="text-gray-600">Admin collections interface will be implemented here.</p>
          </div>
        </div>
      </div>
    `;
  }
}

// Main Alpine.js app data and methods
function app() {
  return {
    // Application state
    user: null,
    initialized: false,
    router: null,
    
    // Initialize the app
    async init() {
      try {
        // Initialize auth manager
        await window.authManager.init();
        this.user = window.authManager.user;
        
        // Initialize router
        this.router = new AlpineRouter();
        
        this.initialized = true;
        console.log('Setagaya app initialized with user:', this.user);
      } catch (error) {
        console.error('Failed to initialize app:', error);
        this.initialized = true; // Still mark as initialized to show UI
      }
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
  }
}

// Make app function globally available
window.shibuyaApp = app;

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